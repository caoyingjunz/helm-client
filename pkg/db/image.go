package db

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/caoyingjunz/rainbow/pkg/db/model"
)

type ImageInterface interface {
	Create(ctx context.Context, object *model.Image) (*model.Image, error)
	Delete(ctx context.Context, imageId int64) error
	Get(ctx context.Context, imageId int64) (*model.Image, error)
	List(ctx context.Context, opts ...Options) ([]model.Image, error)
	ListWithTask(ctx context.Context, taskId int64, opts ...Options) ([]model.Image, error)
}

func newImage(db *gorm.DB) ImageInterface {
	return &image{db}
}

type image struct {
	db *gorm.DB
}

func (a *image) Create(ctx context.Context, object *model.Image) (*model.Image, error) {
	now := time.Now()
	object.GmtCreate = now
	object.GmtModified = now

	if err := a.db.WithContext(ctx).Create(object).Error; err != nil {
		return nil, err
	}
	return object, nil
}

func (a *image) Delete(ctx context.Context, imageId int64) error {
	return nil
}

func (a *image) Get(ctx context.Context, imageId int64) (*model.Image, error) {
	var audit model.Image
	if err := a.db.WithContext(ctx).Where("id = ?", imageId).First(&audit).Error; err != nil {
		return nil, err
	}
	return &audit, nil
}

func (a *image) List(ctx context.Context, opts ...Options) ([]model.Image, error) {
	var audits []model.Image
	tx := a.db.WithContext(ctx)
	for _, opt := range opts {
		tx = opt(tx)
	}
	if err := tx.Find(&audits).Error; err != nil {
		return nil, err
	}

	return audits, nil
}

func (a *image) ListWithTask(ctx context.Context, taskId int64, opts ...Options) ([]model.Image, error) {
	var audits []model.Image
	tx := a.db.WithContext(ctx)
	for _, opt := range opts {
		tx = opt(tx)
	}
	if err := tx.Where("task_id = ?", taskId).Find(&audits).Error; err != nil {
		return nil, err
	}

	return audits, nil
}
