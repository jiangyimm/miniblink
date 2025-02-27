package miniblink

//#include "webview.h"
import "C"
import (
	"github.com/del-xiong/miniblink/eventemitter"
	"github.com/lxn/win"
	"reflect"
	"unsafe"
)

type WebView struct {
	eventemitter.EventEmitter

	window C.wkeWebView
	handle win.HWND

	autoTitle bool
	jsFunc    map[string]interface{}
	jsData    map[string]string

	//事件channels
	DocumentReady chan interface{} //文档ready
	Destroy       chan interface{} //webview销毁

	IsDestroy bool

	blacklist []string // 黑名单url
	// 设置白名单url则认为是强制白名单模式 如果开启此模式 只能访问能够匹配白名单的地址
	whitelist []string
	// 设置内容返回处理回调
	urlEndHandler interface{}
	// 内容处理回调允许处理的内容类型
	urlEndHandlerMimeTypes []string
}

func NewWebView(isTransparent bool, bounds ...int) *WebView {
	view := &WebView{
		autoTitle:     true,
		jsFunc:        make(map[string]interface{}),
		jsData:        make(map[string]string),
		DocumentReady: make(chan interface{}),
		Destroy:       make(chan interface{}),
		IsDestroy:     false,
		urlEndHandlerMimeTypes: []string{
			//"text/html", // html
			//"application/x-javascript", //js
			//"text/css", //css
			//"application/json", //json
			//"image/svg+xml", //svg
		},
	}
	//初始化event emitter
	view.Init()

	width, height, x, y := 800, 600, 200, 200

	if len(bounds) >= 2 {
		width = bounds[0]
		height = bounds[1]
	}

	if len(bounds) >= 4 {
		x = bounds[2]
		y = bounds[3]
	}

	done := make(chan bool)
	jobQueue <- func() {
		view.window = C.createWebWindow(C.bool(isTransparent), C.int(x), C.int(y), C.int(width), C.int(height))
		view.handle = win.HWND(uintptr(unsafe.Pointer(C.getWindowHandle(view.window))))
		close(done)
	}
	<-done

	//初始化各种事件
	//destroy的时候需要设置标志位
	view.On("destroy", func(v *WebView) {
		//关闭destroy,如果已经关闭了,则无需关闭
		select {
		case <-v.Destroy:
			break
		default:
			close(v.Destroy)
		}
		v.IsDestroy = true
	})
	view.On("documentReady", func(v *WebView) {
		select {
		case <-v.DocumentReady:
			break
		default:
			close(v.DocumentReady)
		}
	})
	//同步网页标题到窗口
	view.On("titleChanged", func(view *WebView, title string) {
		if view.autoTitle {
			view.SetWindowTitle(title)
		}
	})

	//注入预置的API给js调用
	view.Inject("MoveToCenter", view.MoveToCenter)
	view.Inject("Move", view.Move)
	view.Inject("GetWindowRect", view.GetWindowRect)
	view.Inject("SetWindowTitle", view.SetWindowTitle)
	view.Inject("EnableAutoTitle", view.EnableAutoTitle)
	view.Inject("DisableAutoTitle", view.DisableAutoTitle)
	view.Inject("ShowDockIcon", view.ShowDockIcon)
	view.Inject("HideDockIcon", view.HideDockIcon)
	view.Inject("ShowWindow", view.ShowWindow)
	view.Inject("HideWindow", view.HideWindow)
	view.Inject("ShowDevTools", view.ShowDevTools)
	view.Inject("ToTop", view.ToTop)
	view.Inject("MostTop", view.MostTop)
	view.Inject("MinimizeWindow", view.MinimizeWindow)
	view.Inject("MaximizeWindow", view.MaximizeWindow)
	view.Inject("RestoreWindow", view.RestoreWindow)
	view.Inject("DestroyWindow", view.DestroyWindow)

	//把webview添加到池中
	addViewToPool(view)
	return view
}

func (view *WebView) processMessage(msg *win.MSG) bool {
	//TODO:临时监听一波键盘事件,并直接处理了,以后要分发到标准的事件中去的
	if isDebug {
		if msg.Message == win.WM_KEYDOWN {
			switch msg.WParam {
			case 0x74: //F5
				go view.Reload()
				break
			case 0x7b: //F12
				go view.ShowDevTools()
				break
			}
		}
	}

	return true
}

func (view *WebView) MoveToCenter() {
	var width int32 = 0
	var height int32 = 0
	{
		rect := &win.RECT{}
		win.GetWindowRect(view.handle, rect)
		width = rect.Right - rect.Left
		height = rect.Bottom - rect.Top
	}

	var parentWidth int32 = 0
	var parentHeight int32 = 0
	if win.GetWindowLong(view.handle, win.GWL_STYLE) == win.WS_CHILD {
		parent := win.GetParent(view.handle)
		rect := &win.RECT{}
		win.GetClientRect(parent, rect)
		parentWidth = rect.Right - rect.Left
		parentHeight = rect.Bottom - rect.Top
	} else {
		parentWidth = win.GetSystemMetrics(win.SM_CXSCREEN)
		parentHeight = win.GetSystemMetrics(win.SM_CYSCREEN)
	}

	x := (parentWidth - width) / 2
	y := (parentHeight - height) / 2

	win.MoveWindow(view.handle, x, y, width, height, false)
}

func (view *WebView) Move(x, y int32, relative bool) {
	var width int32 = 0
	var height int32 = 0
	{
		rect := &win.RECT{}
		win.GetWindowRect(view.handle, rect)
		width = rect.Right - rect.Left
		height = rect.Bottom - rect.Top
		if relative {
			x = rect.Left + x
			y = rect.Top + y
		}
	}
	win.MoveWindow(view.handle, x, y, width, height, false)
}

func (view *WebView) GetWindowRect() (int32, int32) {
	rect := &win.RECT{}
	win.GetWindowRect(view.handle, rect)
	return rect.Left, rect.Top
}

func (view *WebView) SetWindowTitle(title string) {
	done := make(chan bool)
	jobQueue <- func() {
		C.setWindowTitle(view.window, C.CString(title))
		close(done)
	}
	<-done
}

// 设置是否检查跨域 默认true
func (view *WebView) SetCspCheck(enable bool) {
	done := make(chan bool)
	jobQueue <- func() {
		C.setCspCheck(view.window, C._Bool(enable))
		close(done)
	}
	<-done
}

// 设置是否允许新窗口打开 默认true
func (view *WebView) SetNavigationToNewWindowEnable(enable bool) {
	done := make(chan bool)
	jobQueue <- func() {
		C.setNavigationToNewWindowEnable(view.window, C._Bool(enable))
		close(done)
	}
	<-done
}

func (view *WebView) EnableAutoTitle() {
	view.autoTitle = true
	view.SetWindowTitle(view.GetWebTitle())
}

func (view *WebView) DisableAutoTitle() {
	view.autoTitle = false
}

func (view *WebView) GetWebTitle() string {
	//等待document ready,文档没有ready,网页的标题获取不到
	<-view.DocumentReady

	done := make(chan string)
	jobQueue <- func() {
		done <- C.GoString(C.getWebTitle(view.window))
		close(done)
	}
	return <-done
}

func (view *WebView) LoadURL(url string) {
	done := make(chan bool)
	jobQueue <- func() {
		C.loadURL(view.window, C.CString(url))
		close(done)
	}
	<-done
}

func (view *WebView) ShowCaption() {
	style := win.GetWindowLongPtr(view.handle, win.GWL_STYLE)
	win.SetWindowLongPtr(view.handle, win.GWL_STYLE, style|win.WS_CAPTION|win.WS_SYSMENU|win.WS_SIZEBOX)
}

func (view *WebView) HideCaption() {
	style := win.GetWindowLongPtr(view.handle, win.GWL_STYLE)
	win.SetWindowLongPtr(view.handle, win.GWL_STYLE, style&^win.WS_CAPTION&^win.WS_SYSMENU&^win.WS_SIZEBOX)
}

func (view *WebView) ShowDockIcon() {
	style := win.GetWindowLong(view.handle, win.GWL_EXSTYLE)
	win.SetWindowLong(view.handle, win.GWL_EXSTYLE, style&^win.WS_EX_TOOLWINDOW)
}

func (view *WebView) HideDockIcon() {
	style := win.GetWindowLong(view.handle, win.GWL_EXSTYLE)
	win.SetWindowLong(view.handle, win.GWL_EXSTYLE, style|win.WS_EX_TOOLWINDOW)

}

func (view *WebView) ShowWindow() {
	win.ShowWindow(view.handle, win.SW_SHOW)
}

func (view *WebView) HideWindow() {
	win.ShowWindow(view.handle, win.SW_HIDE)
}

func (view *WebView) ShowDevTools() {
	done := make(chan bool)
	jobQueue <- func() {
		C.showDevTools(view.window)
		close(done)
	}
	<-done
}

func (view *WebView) Reload() {
	done := make(chan bool)
	jobQueue <- func() {
		C.reloadURL(view.window)
		close(done)
	}
	<-done
}

func (view *WebView) ToTop() {
	rect := &win.RECT{}
	win.GetWindowRect(view.handle, rect)
	win.SetWindowPos(view.handle, win.HWND_TOP, rect.Left, rect.Top, rect.Right-rect.Left, rect.Bottom-rect.Top, 0)
}

func (view *WebView) MostTop(isTop bool) {
	rect := &win.RECT{}
	win.GetWindowRect(view.handle, rect)
	if isTop {
		win.SetWindowPos(view.handle, win.HWND_TOPMOST, rect.Left, rect.Top, rect.Right-rect.Left, rect.Bottom-rect.Top, 0)
	} else {
		win.SetWindowPos(view.handle, win.HWND_NOTOPMOST, rect.Left, rect.Top, rect.Right-rect.Left, rect.Bottom-rect.Top, 0)
	}
}

func (view *WebView) MaximizeWindow() {
	win.ShowWindow(view.handle, win.SW_MAXIMIZE)
}

func (view *WebView) MinimizeWindow() {
	win.ShowWindow(view.handle, win.SW_MINIMIZE)
}

func (view *WebView) RestoreWindow() {
	win.ShowWindow(view.handle, win.SW_RESTORE)
}

func (view *WebView) DestroyWindow() {
	if !view.IsDestroy {
		done := make(chan bool)
		jobQueue <- func() {
			//关闭destroy,如果已经关闭了,则无需关闭
			select {
			case <-view.Destroy:
				break
			default:
				close(view.Destroy)
			}
			view.IsDestroy = true
			C.destroyWindow(view.window)
			close(done)
		}
		<-done
	}
}
// 设置黑名单
func (view *WebView) SetBlacklist(args ...string) {
	view.blacklist = append(view.blacklist, args...)
}
// 清空黑名单
func (view *WebView) ClearBlacklist() {
	view.blacklist = make([]string, 0)
}
// 从黑名单移除指定路径
func (view *WebView) RemoveFromBlacklist(path string) {
	for k, v := range view.blacklist {
		if v == path {
			view.blacklist = append(view.blacklist[:k], view.blacklist[k+1:]...)
		}
	}
}
// 设置白名单
func (view *WebView) SetWhitelist(args ...string) {
	view.whitelist = append(view.whitelist, args...)
}

// 设置请求处理钩子函数
func (view *WebView) SetUrlEndHandler(value interface{}) {
	if reflect.TypeOf(value).Kind() == reflect.Func {
		view.urlEndHandler = value
	}
}
// 添加urlEndHandlerMimeTypes处理类型
func (view *WebView) AddUrlEndHandlerMimeTypes(args ...string) {
	view.urlEndHandlerMimeTypes = append(view.urlEndHandlerMimeTypes, args...)
}
