package main

import (
	"github.com/complyue/ddgo/pkg/backend"
	"github.com/kataras/iris"
)

func main() {
	app := iris.New()

	// api gateway routes
	backend.DefineHttpRoutes(app)

	// static resources/pages
	app.StaticWeb("/static", "./web/static")

	// use Pongo2 as template engine
	tmpl := iris.Django("./web/templates", ".html")
	tmpl.Reload(true) // reload templates on each request (development mode)
	// register the view engine to the views, this will load the templates.
	app.RegisterView(tmpl)
	// dynamic page routes
	definePageRoutes(app)

	app.Run(iris.Addr(":7070"), iris.WithCharset("UTF-8"))
}
