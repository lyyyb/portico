package launcher

import (
	_ "embed"
	"html/template"
)

//go:embed ui/launcher.html
var launcherPage string

var launcherTemplate = template.Must(template.New("launcher").Parse(launcherPage))
