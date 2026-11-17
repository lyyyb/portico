package launcher

import (
	"net/http"
	"testing"

	"portico/internal/project"
)

func TestBuildPageStartedFirst(t *testing.T) {
	alpha := &project.Project{Key: "alpha", Name: "Alpha"}
	beta := &project.Project{Key: "beta", Name: "Beta"}
	instances := []*instance{
		{p: alpha},
		{p: beta, srv: &http.Server{}, port: 18081, url: "http://127.0.0.1:18081/"},
	}

	data := buildPage(instances, "/tmp/apps", (*instance).live)

	if data.Total != 2 || data.Started != 1 {
		t.Fatalf("统计错误: total=%d started=%d", data.Total, data.Started)
	}
	if !data.Apps[0].Started || data.Apps[0].Name != "Beta" {
		t.Errorf("已启动项目未排在首位: %+v", data.Apps[0])
	}
	if data.Apps[0].Port != 18081 {
		t.Errorf("端口未带入页面: %d", data.Apps[0].Port)
	}
	if data.Apps[0].StopLink != "/stop/beta" {
		t.Errorf("停止链接错误: %s", data.Apps[0].StopLink)
	}
	if data.AppsPath != "/tmp/apps" {
		t.Errorf("目录未带入页面: %s", data.AppsPath)
	}
}

func TestOrderPin(t *testing.T) {
	alpha := &project.Project{Key: "alpha", Name: "Alpha", Pin: true}
	beta := &project.Project{Key: "beta", Name: "Beta", Order: 10}
	gamma := &project.Project{Key: "gamma", Name: "Gamma"}
	instances := []*instance{{p: alpha}, {p: beta}, {p: gamma}}

	data := buildPage(instances, "/tmp/apps", (*instance).live)

	got := []string{data.Apps[0].Name, data.Apps[1].Name, data.Apps[2].Name}
	want := []string{"Alpha", "Beta", "Gamma"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序错误: got %v, want %v", got, want)
		}
	}
	if !data.Apps[0].Pin || data.Apps[1].Pin {
		t.Error("置顶标记未带入页面")
	}
}

func TestInitialAndHue(t *testing.T) {
	if got := initial("订单系统"); got != "订" {
		t.Errorf("initial 取首字错误: %q", got)
	}
	if got := initial(""); got != "?" {
		t.Errorf("空名称应返回 ?，实际 %q", got)
	}
	if hue("订单系统") != hue("订单系统") {
		t.Error("同名项目的色相应保持稳定")
	}
}
