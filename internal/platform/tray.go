package platform

// TrayApp 是托盘菜单里可单独操作的应用
type TrayApp struct {
	Key  string
	Name string
}

// TrayHandlers 是托盘需要回调的动作
type TrayHandlers struct {
	Open    func()           // 打开启动器页面
	Running func() []TrayApp // 当前运行中的应用
	OpenApp func(key string) // 打开某个运行中应用的页面
	StopApp func(key string) // 停止某个应用
	StopAll func()           // 停止全部应用
	OpenLog func()           // 打开日志文件
	About   func()           // 关于
}
