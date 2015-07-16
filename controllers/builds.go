package controllers

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/convox/kernel/Godeps/_workspace/src/github.com/ddollar/logger"
	"github.com/convox/kernel/Godeps/_workspace/src/github.com/gorilla/mux"
	"github.com/convox/kernel/Godeps/_workspace/src/github.com/gorilla/websocket"

	"github.com/convox/kernel/helpers"
	"github.com/convox/kernel/models"
)

func BuildList(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("list").Start()

	vars := mux.Vars(r)
	app := vars["app"]

	l := map[string]string{
		"id":      r.URL.Query().Get("id"),
		"created": r.URL.Query().Get("created"),
	}

	builds, err := models.ListBuilds(app, l)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	a, err := models.GetApp(app)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	params := map[string]interface{}{
		"App":    a,
		"Builds": builds,
	}

	if len(builds) > 0 {
		params["Last"] = builds[len(builds)-1]
	}

	switch r.Header.Get("Content-Type") {
	case "application/json":
		RenderJson(rw, builds)
	default:
		RenderPartial(rw, "app", "builds", params)
	}
}

func BuildGet(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("list").Start()

	vars := mux.Vars(r)
	app := vars["app"]
	build := vars["build"]

	b, err := models.GetBuild(app, build)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	RenderJson(rw, b)
}

func BuildCreate(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("create").Start()

	err := r.ParseMultipartForm(10485760)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	app := mux.Vars(r)["app"]

	build := models.NewBuild(app)

	err = build.Save()

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	log.Success("step=build.save app=%q", build.App)

	if r.MultipartForm != nil && r.MultipartForm.File["source"] != nil {
		fd, err := r.MultipartForm.File["source"][0].Open()

		if err != nil {
			helpers.Error(log, err)
			RenderError(rw, err)
			return
		}

		defer fd.Close()

		dir, err := ioutil.TempDir("", "source")

		if err != nil {
			helpers.Error(log, err)
			RenderError(rw, err)
			return
		}

		err = os.MkdirAll(dir, 0755)

		if err != nil {
			helpers.Error(log, err)
			RenderError(rw, err)
			return
		}

		go build.ExecuteLocal(fd)
	} else if repo := GetForm(r, "repo"); repo != "" {
		go build.ExecuteRemote(repo)
	} else {
		err = fmt.Errorf("no source or repo")
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	RenderText(rw, build.Id)
}

func BuildLogs(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("logs").Start()

	vars := mux.Vars(r)
	app := vars["app"]
	id := vars["build"]

	build, err := models.GetBuild(app, id)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	log.Success("step=build.logs app=%q", build.App)

	RenderText(rw, build.Logs)
}

func BuildStatus(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("status").Start()

	vars := mux.Vars(r)
	app := vars["app"]
	id := vars["build"]

	build, err := models.GetBuild(app, id)

	if err != nil {
		helpers.Error(log, err)
		RenderError(rw, err)
		return
	}

	RenderText(rw, build.Status)
}

func BuildStream(rw http.ResponseWriter, r *http.Request) {
	log := buildsLogger("stream").Start()

	vars := mux.Vars(r)
	app := vars["app"]
	id := vars["build"]

	ws, err := upgrader.Upgrade(rw, r, nil)

	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		helpers.Error(log, err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("error: %s", err)))
		return
	}

	defer ws.Close()

	sent := ""

	for {
		b, err := models.GetBuild(app, id)

		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
			helpers.Error(log, err)
			RenderError(rw, err)
			return
		}

		ws.WriteMessage(websocket.TextMessage, []byte(b.Logs[len(sent):]))

		sent = b.Logs

		switch b.Status {
		case "complete", "failed":
			break
		}

		time.Sleep(1 * time.Second)
	}
}

func buildsLogger(at string) *logger.Logger {
	return logger.New("ns=kernel cn=builds").At(at)
}
