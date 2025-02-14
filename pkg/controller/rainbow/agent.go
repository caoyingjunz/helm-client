package rainbow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/caoyingjunz/pixiulib/strutil"
	rainbowconfig "github.com/caoyingjunz/rainbow/cmd/app/config"
	"github.com/caoyingjunz/rainbow/pkg/db"
	"github.com/caoyingjunz/rainbow/pkg/db/model"
	"github.com/caoyingjunz/rainbow/pkg/template"
	"github.com/caoyingjunz/rainbow/pkg/util"
	"github.com/caoyingjunz/rainbow/pkg/util/errors"
)

type AgentGetter interface {
	Agent() Interface
}
type Interface interface {
	Run(ctx context.Context, workers int) error
}

type AgentController struct {
	factory db.ShareDaoFactory
	cfg     rainbowconfig.Config

	queue workqueue.RateLimitingInterface

	name     string
	callback string
	baseDir  string
}

func NewAgent(f db.ShareDaoFactory, cfg rainbowconfig.Config) *AgentController {
	return &AgentController{
		factory:  f,
		cfg:      cfg,
		name:     cfg.Agent.Name,
		baseDir:  cfg.Agent.DataDir,
		callback: cfg.Plugin.Callback,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "rainbow-agent"),
	}
}

func (s *AgentController) Run(ctx context.Context, workers int) error {
	// 注册 rainbow 代理
	if err := s.RegisterAgentIfNotExist(ctx); err != nil {
		return err
	}

	go s.report(ctx)

	go s.getNextWorkItems(ctx)

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, s.worker, 1*time.Second)
	}

	return nil
}

func (s *AgentController) report(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		newAgent, err := s.factory.Agent().GetByName(ctx, s.name)
		if err != nil {
			klog.Error("failed to get agent status %v", err)
			continue
		}

		updates := map[string]interface{}{"last_transition_time": time.Now()}
		if newAgent.Status == model.UnknownAgentType {
			updates["status"] = model.RunAgentType
			updates["message"] = "Agent started posting status"
		}

		err = s.factory.Agent().UpdateByName(ctx, s.name, updates)
		if err != nil {
			klog.Error("failed to sync agent status %v", err)
		}
	}
}

func (s *AgentController) getNextWorkItems(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// 获取未处理
		tasks, err := s.factory.Task().ListWithAgent(ctx, s.name, 0)
		if err != nil {
			klog.Error("failed to list tasks %v", err)
			continue
		}
		if len(tasks) == 0 {
			continue
		}

		for _, task := range tasks {
			s.queue.Add(fmt.Sprintf("%d/%d", task.Id, task.ResourceVersion))
		}
	}
}

func (s *AgentController) worker(ctx context.Context) {
	for s.processNextWorkItem(ctx) {
	}
}

func (s *AgentController) processNextWorkItem(ctx context.Context) bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(key)

	taskId, resourceVersion, err := KeyFunc(key)
	if err != nil {
		s.handleErr(ctx, err, key)
	} else {
		s.handleErr(ctx, s.sync(ctx, taskId, resourceVersion), key)
	}
	return true
}

func (s *AgentController) makePluginConfig(ctx context.Context, task model.Task) (*template.PluginTemplateConfig, error) {
	taskId := task.Id

	registry, err := s.factory.Registry().Get(ctx, task.RegisterId)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry %v", err)
	}
	images, err := s.factory.Image().ListWithTask(ctx, taskId)
	if err != nil {
		return nil, fmt.Errorf("failed to get images %v", err)
	}
	var img []string
	for _, image := range images {
		img = append(img, image.Name)
	}

	return &template.PluginTemplateConfig{
		Default: template.DefaultOption{
			PushImages: true,
		},
		Plugin: template.PluginOption{
			Callback: s.callback,
			TaskId:   taskId,
			Synced:   true,
		},
		Registry: template.Registry{
			Repository: registry.Repository,
			Namespace:  registry.Namespace,
			Username:   registry.Username,
			Password:   registry.Password,
		},
		Images: img,
	}, nil
}

func (s *AgentController) sync(ctx context.Context, taskId int64, resourceVersion int64) error {
	task, err := s.factory.Task().GetOne(ctx, taskId, resourceVersion)
	if err != nil {
		if errors.IsNotUpdated(err) {
			return nil
		}
		return fmt.Errorf("failted to get one task %d %v", taskId, err)
	}

	tplCfg, err := s.makePluginConfig(ctx, *task)
	cfg, err := yaml.Marshal(tplCfg)
	if err != nil {
		return err
	}

	taskIdStr := fmt.Sprintf("%d", taskId)

	destDir := filepath.Join(s.baseDir, taskIdStr)
	if err = util.EnsureDirectoryExists(destDir); err != nil {
		return err
	}
	if !util.IsDirectoryExists(destDir + "/plugin") {
		if err = util.Copy(s.baseDir+"/plugin", destDir); err != nil {
			return err
		}
	}

	git := util.NewGit(destDir+"/plugin", taskIdStr, taskIdStr+"-"+time.Now().String())
	if err = git.Checkout(); err != nil {
		return err
	}
	if err = util.WriteIntoFile(string(cfg), destDir+"/plugin/config.yaml"); err != nil {
		return err
	}
	if err = git.Push(); err != nil {
		return err
	}
	return nil
}

// TODO
func (s *AgentController) handleErr(ctx context.Context, err error, key interface{}) {
	if err == nil {
		return
	}
	klog.Error(err)
}

func (s *AgentController) RegisterAgentIfNotExist(ctx context.Context) error {
	if len(s.name) == 0 {
		return fmt.Errorf("agent name missing")
	}

	var err error
	_, err = s.factory.Agent().GetByName(ctx, s.name)
	if err == nil {
		return nil
	}
	_, err = s.factory.Agent().Create(ctx, &model.Agent{Name: s.name, Status: model.RunAgentType, Type: model.PublicAgentType, Message: "Agent started posting status"})
	return err
}

func KeyFunc(key interface{}) (int64, int64, error) {
	str, ok := key.(string)
	if !ok {
		return 0, 0, fmt.Errorf("failed to convert %v to string", key)
	}
	parts := strings.Split(str, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("parts length not 2")
	}

	taskId, err := strutil.ParseInt64(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to Parse taskId to Int64 %v", err)
	}
	resourceVersion, err := strutil.ParseInt64(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to Parse resourceVersion to Int64 %v", err)
	}

	return taskId, resourceVersion, nil
}
