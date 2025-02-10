// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"errors"
	"net/http"

	actions_model "code.gitea.io/gitea/models/actions"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/web"
	shared "code.gitea.io/gitea/routers/web/shared/actions"
	shared_user "code.gitea.io/gitea/routers/web/shared/user"
	actions_service "code.gitea.io/gitea/services/actions"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/forms"
)

const (
	tplRepoVariables  templates.TplName = "repo/settings/actions"
	tplOrgVariables   templates.TplName = "org/settings/actions"
	tplUserVariables  templates.TplName = "user/settings/actions"
	tplAdminVariables templates.TplName = "admin/actions"
)

type variablesCtx struct {
	OwnerID           int64
	RepoID            int64
	IsRepo            bool
	IsOrg             bool
	IsUser            bool
	IsGlobal          bool
	VariablesTemplate templates.TplName
	RedirectLink      string
}

func getVariablesCtx(ctx *context.Context) (*variablesCtx, error) {
	if ctx.Data["PageIsRepoSettings"] == true {
		return &variablesCtx{
			OwnerID:           0,
			RepoID:            ctx.Repo.Repository.ID,
			IsRepo:            true,
			VariablesTemplate: tplRepoVariables,
			RedirectLink:      ctx.Repo.RepoLink + "/settings/actions/variables",
		}, nil
	}

	if ctx.Data["PageIsOrgSettings"] == true {
		err := shared_user.LoadHeaderCount(ctx)
		if err != nil {
			ctx.ServerError("LoadHeaderCount", err)
			return nil, nil
		}
		return &variablesCtx{
			OwnerID:           ctx.ContextUser.ID,
			RepoID:            0,
			IsOrg:             true,
			VariablesTemplate: tplOrgVariables,
			RedirectLink:      ctx.Org.OrgLink + "/settings/actions/variables",
		}, nil
	}

	if ctx.Data["PageIsUserSettings"] == true {
		return &variablesCtx{
			OwnerID:           ctx.Doer.ID,
			RepoID:            0,
			IsUser:            true,
			VariablesTemplate: tplUserVariables,
			RedirectLink:      setting.AppSubURL + "/user/settings/actions/variables",
		}, nil
	}

	if ctx.Data["PageIsAdmin"] == true {
		return &variablesCtx{
			OwnerID:           0,
			RepoID:            0,
			IsGlobal:          true,
			VariablesTemplate: tplAdminVariables,
			RedirectLink:      setting.AppSubURL + "/-/admin/actions/variables",
		}, nil
	}

	return nil, errors.New("unable to set Variables context")
}

func Variables(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("actions.variables")
	ctx.Data["PageType"] = "variables"
	ctx.Data["PageIsSharedSettingsVariables"] = true

	vCtx, err := getVariablesCtx(ctx)
	if err != nil {
		ctx.ServerError("getVariablesCtx", err)
		return
	}

	shared.SetVariablesContext(ctx, vCtx.OwnerID, vCtx.RepoID)
	if ctx.Written() {
		return
	}

	ctx.HTML(http.StatusOK, vCtx.VariablesTemplate)
}

func VariableCreate(ctx *context.Context) {
	vCtx, err := getVariablesCtx(ctx)
	if err != nil {
		ctx.ServerError("getVariablesCtx", err)
		return
	}

	if ctx.HasError() { // form binding validation error
		ctx.JSONError(ctx.GetErrMsg())
		return
	}

	shared.CreateVariable(ctx, vCtx.OwnerID, vCtx.RepoID, vCtx.RedirectLink)
}

func VariableUpdate(ctx *context.Context) {
	vCtx, err := getVariablesCtx(ctx)
	if err != nil {
		ctx.ServerError("getVariablesCtx", err)
		return
	}

	if ctx.HasError() { // form binding validation error
		ctx.JSONError(ctx.GetErrMsg())
		return
	}

	id := ctx.PathParamInt64("variable_id")

	opts := actions_model.FindVariablesOpts{
		IDs: []int64{id},
	}
	switch {
	case vCtx.IsRepo:
		opts.RepoID = vCtx.RepoID
	case vCtx.IsOrg:
		opts.OwnerID = vCtx.OwnerID
	case vCtx.IsUser:
		opts.OwnerID = vCtx.OwnerID
	case vCtx.IsGlobal:
		// do nothing
	}

	var variable *actions_model.ActionVariable
	if got, err := actions_model.FindVariables(ctx, opts); err != nil {
		ctx.ServerError("FindVariables", err)
		return
	} else if len(got) == 0 {
		ctx.NotFound("FindVariables", nil)
		return
	} else {
		variable = got[0]
	}

	form := web.GetForm(ctx).(*forms.EditVariableForm)
	variable.Name = form.Name
	variable.Data = form.Data

	if ok, err := actions_service.UpdateVariable(ctx, variable); err != nil || !ok {
		log.Error("UpdateVariable: %v", err)
		ctx.JSONError(ctx.Tr("actions.variables.update.failed"))
		return
	}
	ctx.Flash.Success(ctx.Tr("actions.variables.update.success"))
	ctx.JSONRedirect(vCtx.RedirectLink)
}

func VariableDelete(ctx *context.Context) {
	vCtx, err := getVariablesCtx(ctx)
	if err != nil {
		ctx.ServerError("getVariablesCtx", err)
		return
	}
	shared.DeleteVariable(ctx, vCtx.RedirectLink)
}
