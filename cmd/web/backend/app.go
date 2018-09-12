package main

import "github.com/kataras/iris"

func indexPage(ctx iris.Context) {
	ctx.ViewData("Title", "Despatch & Delivery")
	ctx.View("index.html")
}

func plotPage(ctx iris.Context) {
	ctx.ViewData("Title", "plot - Despatch & Delivery")
	tid := ctx.Params().GetTrim("tid")
	ctx.ViewData("tid", tid)
	ctx.View("plot.html")
}

func definePageRoutes(app *iris.Application) {
	app.Get("/", indexPage)
	app.Get("/{tid:string}/plot", plotPage)
}
