package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/caoyingjunz/rainbow/pkg/db/model"
)

type TaskInterface interface {
	Create(ctx context.Context, object *model.Task) (*model.Task, error)
	Update(ctx context.Context, taskId int64, resourceVersion int64, updates map[string]interface{}) error
	Delete(ctx context.Context, taskId int64) error
	Get(ctx context.Context, taskId int64) (*model.Task, error)
	List(ctx context.Context, opts ...Options) ([]model.Task, error)

	AssignToAgent(ctx context.Context, taskId int64, agentName string) error
	ListWithAgent(ctx context.Context, agentName string, opts ...Options) ([]model.Task, error)
}

func newTask(db *gorm.DB) TaskInterface {
	return &task{db}
}

type task struct {
	db *gorm.DB
}

func (a *task) Create(ctx context.Context, object *model.Task) (*model.Task, error) {
	now := time.Now()
	object.GmtCreate = now
	object.GmtModified = now

	if err := a.db.WithContext(ctx).Create(object).Error; err != nil {
		return nil, err
	}
	return object, nil
}

func (a *task) Update(ctx context.Context, taskId int64, resourceVersion int64, updates map[string]interface{}) error {
	updates["gmt_modified"] = time.Now()
	updates["resource_version"] = resourceVersion + 1

	f := a.db.WithContext(ctx).Model(&model.Task{}).Where("id = ? and resource_version = ?", taskId, resourceVersion).Updates(updates)
	if f.Error != nil {
		return f.Error
	}
	if f.RowsAffected == 0 {
		return fmt.Errorf("record not updated")
	}

	return nil
}

func (a *task) Delete(ctx context.Context, taskId int64) error {
	return nil
}

func (a *task) Get(ctx context.Context, agentId int64) (*model.Task, error) {
	var audit model.Task
	if err := a.db.WithContext(ctx).Where("id = ?", agentId).First(audit).Error; err != nil {
		return nil, err
	}
	return &audit, nil
}

func (a *task) List(ctx context.Context, opts ...Options) ([]model.Task, error) {
	var audits []model.Task
	tx := a.db.WithContext(ctx)
	for _, opt := range opts {
		tx = opt(tx)
	}
	if err := tx.Find(&audits).Error; err != nil {
		return nil, err
	}

	return audits, nil
}

func (a *task) ListWithAgent(ctx context.Context, agentName string, opts ...Options) ([]model.Task, error) {
	var audits []model.Task
	tx := a.db.WithContext(ctx)
	for _, opt := range opts {
		tx = opt(tx)
	}
	if err := tx.Where("agent_name = ?", agentName).Find(&audits).Error; err != nil {
		return nil, err
	}

	return audits, nil
}

func (a *task) AssignToAgent(ctx context.Context, taskId int64, agentName string) error {
	f := a.db.WithContext(ctx).Model(&model.Task{}).Where("id = ?", taskId).Updates(map[string]interface{}{
		"gmt_modified": time.Now(),
		"agent_name":   agentName,
	})

	return f.Error
}
