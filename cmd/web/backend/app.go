package main

import (
	"fmt"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/flosch/pongo2"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"net/http"
)

func definePageRoutes(router *mux.Router) {

	router.Handle("/", &Pongo2Page{
		TmplFile: "web/templates/index.html",
		Ctx: pongo2.Context{
			"title": "Despatch & Delivery",
		},
	})

	router.Handle("/{tid}/plot", &Pongo2Page{
		TmplFile: "web/templates/plot.html",
		Ctx: pongo2.Context{
			"title": "plot - Despatch & Delivery",
		},
	})

}

type Pongo2Page struct {
	TmplFile string
	Ctx      pongo2.Context
	tmpl     *pongo2.Template
}

func (page *Pongo2Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			http.Error(w, fmt.Sprintf("%+v", err), http.StatusInternalServerError)
		}
	}()
	if page.tmpl == nil || devMode {
		page.tmpl = pongo2.Must(pongo2.FromFile(page.TmplFile))
	}
	ctx := make(pongo2.Context)
	for k, v := range page.Ctx {
		ctx[k] = v
	}
	for k, v := range mux.Vars(r) {
		ctx[k] = v
	}
	err = page.tmpl.ExecuteWriter(ctx, w)
	if err != nil {
		return
	}
}
