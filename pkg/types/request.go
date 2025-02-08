package types

type (
	CreateTaskRequest struct {
		UserId     int64    `json:"user_id"`
		RegisterId int64    `json:"register_id"`
		Images     []string `json:"images"`
		AgentName  string   `json:"agent_name"`
	}

	CreateRegistryRequest struct {
		UserId     string `json:"user_id"`
		Repository string `json:"repository"`
		Namespace  string `json:"namespace"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}

	CreateImageRequest struct {
		TaskId  int64  `json:"task_id"`
		Name    string `json:"name"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	UpdateImageRequest struct {
		Id              int64  `json:"id"`
		TaskId          int64  `json:"task_id"`
		ResourceVersion int64  `json:"resource_version"`
		Name            string `json:"name"`
		Status          string `json:"status"`
		Message         string `json:"message"`
	}
)
