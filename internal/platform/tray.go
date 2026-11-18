package platform

// TrayApp 是托盘菜单里可单独操作的应用
type TrayApp struct {
	Key  string
	Name string
	Web  bool // true 表示静态站点，打开的是网页；false 表示桌面程序，切换到已有窗口
}

// TrayHandlers 是托盘需要回调的动作
type TrayHandlers struct {
	Open    func()           // 打开启动器页面
	Running func() []TrayApp // 当前运行中的应用
	OpenApp func(key string) // 打开或显示某个运行中应用
	StopApp func(key string) // 停止某个应用
	StopAll func()           // 停止全部应用
	OpenLog func()           // 打开日志文件
	About   func()           // 关于
}
