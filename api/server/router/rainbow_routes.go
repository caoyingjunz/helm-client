package router

import (
	"github.com/caoyingjunz/pixiulib/httputils"
	"github.com/caoyingjunz/rainbow/pkg/types"
	"github.com/gin-gonic/gin"
)

func (cr *rainbowRouter) createTask(c *gin.Context) {
	r := httputils.NewResponse()
	httputils.SetSuccess(c, r)
}

func (cr *rainbowRouter) updateTask(c *gin.Context) {}

func (cr *rainbowRouter) deleteTask(c *gin.Context) {}

func (cr *rainbowRouter) getTask(c *gin.Context) {}

func (cr *rainbowRouter) listTasks(c *gin.Context) {}

func (cr *rainbowRouter) createRegistry(c *gin.Context) {
	resp := httputils.NewResponse()

	var (
		req types.CreateRegistryRequest
		err error
	)
	if err = httputils.ShouldBindAny(c, &req, nil, nil); err != nil {
		httputils.SetFailed(c, resp, err)
		return
	}
	if err = cr.c.Server().CreateRegistry(c, &req); err != nil {
		httputils.SetFailed(c, resp, err)
		return
	}

	httputils.SetSuccess(c, resp)
}

func (cr *rainbowRouter) updateRegistry(c *gin.Context) {}

func (cr *rainbowRouter) deleteRegistry(c *gin.Context) {}

func (cr *rainbowRouter) getRegistry(c *gin.Context) {}

func (cr *rainbowRouter) listRegistries(c *gin.Context) {}
