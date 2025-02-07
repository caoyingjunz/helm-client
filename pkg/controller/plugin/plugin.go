package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/caoyingjunz/pixiulib/exec"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"k8s.io/klog/v2"

	"github.com/caoyingjunz/rainbow/cmd/app/config"
	"github.com/caoyingjunz/rainbow/pkg/util"
)

const (
	Kubeadm   = "kubeadm"
	IgnoreKey = "W0508"
)

type KubeadmVersion struct {
	ClientVersion struct {
		GitVersion string `json:"gitVersion"`
	} `json:"clientVersion"`
}

type KubeadmImage struct {
	Images []string `json:"images"`
}

type PluginController struct {
	KubernetesVersion string
	Callback          string

	httpClient util.HttpInterface
	exec       exec.Interface
	docker     *client.Client

	Cfg      config.Config
	Registry config.Registry
}

func (img *PluginController) Validate() error {
	if img.Cfg.Default.PushKubernetes {
		if len(img.KubernetesVersion) == 0 {
			return fmt.Errorf("failed to find kubernetes version")
		}
		// 检查 kubeadm 的版本是否和 k8s 版本一致
		kubeadmVersion, err := img.getKubeadmVersion()
		if err != nil {
			return fmt.Errorf("failed to get kubeadm version: %v", err)
		}
		if kubeadmVersion != img.KubernetesVersion {
			return fmt.Errorf("kubeadm version %s not match kubernetes version %s", kubeadmVersion, img.KubernetesVersion)
		}
	}

	// 检查 docker 的客户端是否正常
	//if _, err := img.docker.Ping(context.Background()); err != nil {
	//	return err
	//}

	return nil
}

func (img *PluginController) Complete() error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	img.docker = cli

	if img.Cfg.Default.PushKubernetes {
		if len(img.KubernetesVersion) == 0 {
			if len(img.Cfg.Kubernetes.Version) != 0 {
				img.KubernetesVersion = img.Cfg.Kubernetes.Version
			} else {
				img.KubernetesVersion = os.Getenv("KubernetesVersion")
			}
		}
	}

	if img.Cfg.Default.PushKubernetes {
		//cmd := []string{"sudo", "apt-get", "install", "-y", fmt.Sprintf("kubeadm=%s-00", img.Cfg.Kubernetes.Version[1:])}
		cmd := []string{"sudo", "curl", "-LO", fmt.Sprintf("https://dl.k8s.io/release/%s/bin/linux/amd64/kubeadm", img.Cfg.Kubernetes.Version)}
		klog.Infof("Starting install kubeadm", cmd)
		out, err := img.exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get kubeadm %v %v", string(out), err)
		}

		cmd2 := []string{"sudo", "install", "-o", "root", "-g", "root", "-m", "0755", "kubeadm", "/usr/local/bin/kubeadm"}
		out, err = img.exec.Command(cmd2[0], cmd2[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install kubeadm %v %v", string(out), err)
		}
	}

	img.exec = exec.New()
	img.httpClient = util.NewHttpClient(5*time.Second, img.Callback)
	return nil
}

func (img *PluginController) Close() {
	if img.docker != nil {
		_ = img.docker.Close()
	}
}

func (img *PluginController) getKubeadmVersion() (string, error) {
	if _, err := img.exec.LookPath(Kubeadm); err != nil {
		return "", fmt.Errorf("failed to find %s %v", Kubeadm, err)
	}

	cmd := []string{Kubeadm, "version", "-o", "json"}
	out, err := img.exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to exec kubeadm version %v %v", string(out), err)
	}

	var kubeadmVersion KubeadmVersion
	if err := json.Unmarshal(out, &kubeadmVersion); err != nil {
		return "", fmt.Errorf("failed to unmarshal kubeadm version %v", err)
	}
	klog.V(2).Infof("kubeadmVersion %+v", kubeadmVersion)

	return kubeadmVersion.ClientVersion.GitVersion, nil
}

func (img *PluginController) cleanImages(in []byte) []byte {
	inStr := string(in)
	if !strings.Contains(inStr, IgnoreKey) {
		return in
	}

	klog.V(2).Infof("cleaning images: %+v", inStr)
	parts := strings.Split(inStr, "\n")
	index := 0
	for _, p := range parts {
		if strings.HasPrefix(p, IgnoreKey) {
			index += 1
		}
	}
	newInStr := strings.Join(parts[index:], "\n")
	klog.V(2).Infof("cleaned images: %+v", newInStr)

	return []byte(newInStr)
}

func (img *PluginController) getImages() ([]string, error) {
	cmd := []string{Kubeadm, "config", "images", "list", "--kubernetes-version", img.KubernetesVersion, "-o", "json"}
	out, err := img.exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to exec kubeadm config images list %v %v", string(out), err)
	}
	out = img.cleanImages(out)
	klog.V(2).Infof("images is %+v", string(out))

	var kubeadmImage KubeadmImage
	if err := json.Unmarshal(out, &kubeadmImage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubeadm images %v", err)
	}

	return kubeadmImage.Images, nil
}

func (img *PluginController) parseTargetImage(imageToPush string) (string, error) {
	// real image to push
	parts := strings.Split(imageToPush, "/")

	return img.Registry.Repository + "/" + img.Registry.Namespace + "/" + parts[len(parts)-1], nil
}

func (img *PluginController) doPushImage(imageToPush string) error {
	targetImage, err := img.parseTargetImage(imageToPush)
	if err != nil {
		return err
	}

	klog.Infof("starting pull image %s", imageToPush)
	// start pull
	reader, err := img.docker.ImagePull(context.TODO(), imageToPush, types.ImagePullOptions{})
	if err != nil {
		klog.Errorf("failed to pull %s: %v", imageToPush, err)
		return err
	}
	io.Copy(os.Stdout, reader)

	klog.Infof("tag %s to %s", imageToPush, targetImage)
	if err := img.docker.ImageTag(context.TODO(), imageToPush, targetImage); err != nil {
		klog.Errorf("failed to tag %s to %s: %v", imageToPush, targetImage, err)
		return err
	}

	klog.Infof("starting push image %s", targetImage)

	cmd := []string{"docker", "push", targetImage}
	out, err := img.exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push image %s %v %v", targetImage, string(out), err)
	}

	klog.Infof("complete push image %s", imageToPush)
	return nil
}
func (img *PluginController) getImagesFromFile() ([]string, error) {
	var imgs []string
	for _, i := range img.Cfg.Images {
		imageStr := strings.TrimSpace(i)
		if len(imageStr) == 0 {
			continue
		}
		if strings.Contains(imageStr, " ") {
			return nil, fmt.Errorf("error image format: %s", imageStr)
		}

		imgs = append(imgs, imageStr)
	}

	return imgs, nil
}

func (img *PluginController) Run() error {
	var images []string

	if img.Cfg.Default.PushKubernetes {
		kubeImages, err := img.getImages()
		if err != nil {
			return fmt.Errorf("获取 k8s 镜像失败: %v", err)
		}
		images = append(images, kubeImages...)
	}

	if img.Cfg.Default.PushImages {
		fileImages, err := img.getImagesFromFile()
		if err != nil {
			return fmt.Errorf("")
		}
		images = append(images, fileImages...)
	}

	klog.V(2).Infof("get images: %v", images)
	diff := len(images)
	errCh := make(chan error, diff)

	// 登陆
	cmd := []string{"docker", "login", "-u", img.Registry.Username, "-p", img.Registry.Password}
	if img.Registry.Repository != "" {
		cmd = append(cmd, img.Registry.Repository)
	}
	out, err := img.exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login in image %v %v", string(out), err)
	}

	var wg sync.WaitGroup
	wg.Add(diff)
	for _, i := range images {
		go func(imageToPush string) {
			defer wg.Done()
			if err := img.doPushImage(imageToPush); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	default:
	}

	return nil
}

func (img *PluginController) ReportStatus() error {
	return nil
}
