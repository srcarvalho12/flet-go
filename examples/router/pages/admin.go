package pages

import (
	flet "github.com/srcarvalho12/flet-go"
	"github.com/srcarvalho12/flet-go/colors"
)

var user = flet.NewSessionKey[string](nil)

// RequireLogin sends visitors who are not logged in to /login.
func RequireLogin(next flet.PageFunc) flet.PageFunc {
	return func(ctx *flet.RouteContext) flet.Control {
		if user.Get(ctx.Page) == "" {
			ctx.Redirect("/login?next=" + ctx.Path)
			return nil
		}
		return next(ctx)
	}
}

// AdminLayout is a different frame for the admin area.
func AdminLayout(ctx *flet.RouteContext, content flet.Control) *flet.View {
	return &flet.View{
		AppBar: &flet.AppBar{
			Title:   "Admin · " + user.Get(ctx.Page),
			Bgcolor: colors.Red900,
			Actions: []flet.Control{
				&flet.TextButton{Content: "Log out", OnClick: func(*flet.Event) {
					user.Set(ctx.Page, "")
					ctx.Go("/")
				}},
			},
		},
		Padding:  20,
		Controls: []flet.Control{content},
	}
}

// Dashboard is the protected admin page.
func Dashboard(ctx *flet.RouteContext) flet.Control {
	return &flet.Text{Value: "Only logged-in users see this.", Size: 20}
}

// Login has no layout.
func Login(ctx *flet.RouteContext) flet.Control {
	name := &flet.TextField{Label: "Name"}
	return &flet.Column{Controls: []flet.Control{
		&flet.Text{Value: "Log in", Size: 28},
		name,
		&flet.Button{Content: "Log in", OnClick: func(*flet.Event) {
			if name.Value == "" {
				return
			}
			user.Set(ctx.Page, name.Value)
			next := ctx.Query.Get("next")
			if next == "" {
				next = "/admin"
			}
			ctx.Go(next)
		}},
	}}
}
