// Command tcc-desktop is the Terran CC desktop app: dashboards, the side
// widget, pop-out widgets and a tray icon. It holds no data of its own; every
// window talks to tccd, which it starts if needed.
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"terrancc/internal/client"
	"terrancc/internal/config"
)

//go:embed all:frontend/dist
var dist embed.FS

//go:embed tray.png
var trayIcon []byte

func tccdPath() string {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "tccd.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("tccd"); err == nil {
		return p
	}
	return "tccd"
}

func fatal(msg string) {
	log.Print(msg)
	messageBox("Terran CC", msg) // no console in a windowsgui build
	os.Exit(1)
}

func main() {
	home, err := config.Home()
	if err != nil {
		fatal(err.Error())
	}
	if f, err := os.OpenFile(filepath.Join(home, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
	if _, err := client.EnsureDaemon(home, tccdPath(), 10*time.Second); err != nil {
		fatal("Terran CC could not start its service:\n\n" + err.Error())
	}
	d, err := config.ReadDaemon(home)
	if err != nil {
		fatal(err.Error())
	}
	s := newShell("http://"+d.Addr, d.Token)

	assets, _ := fs.Sub(dist, "frontend/dist")
	notifier := notifications.New()
	app := application.New(application.Options{
		Name:        "Terran CC",
		Description: "Tickets, to-dos, habits and a Pomodoro timer",
		Services:    []application.Service{application.NewService(notifier)},
		Assets: application.AssetOptions{
			Handler:        application.BundledAssetFileServer(assets),
			Middleware:     s.middleware,
			DisableLogging: true,
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "dev.terran-cc.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				s.focusDashboard()
			},
		},
	})
	s.app = app

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("Terran CC")
	menu := app.NewMenu()
	menu.Add("Dashboard").OnClick(func(*application.Context) { s.focusDashboard() })
	menu.Add("New dashboard window").OnClick(func(*application.Context) { s.open(winState{Kind: "dashboard"}) })
	menu.Add("Side widget").OnClick(func(*application.Context) { s.open(winState{Kind: "side"}) })
	menu.AddSeparator()
	menu.Add("Start / pause timer").OnClick(func(*application.Context) {
		if resp, err := s.apiReq("POST", "/api/timer/toggle", nil); err == nil {
			resp.Body.Close()
		}
	})
	menu.AddSeparator()
	menu.Add("Quit Terran CC").OnClick(func(*application.Context) { s.quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { s.focusDashboard() })

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		s.restore()
		go s.watch(tray, notifier)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type snapshot struct {
	Phase     string `json:"phase"`
	State     string `json:"state"`
	Remaining int    `json:"remaining_s"`
}

var phaseNames = map[string]string{"focus": "Focus", "short_break": "Short break", "long_break": "Long break"}

// watch follows tccd's event stream: it keeps the tray tooltip on the timer
// and raises a notification when a phase ends. It reconnects forever.
func (s *shell) watch(tray *application.SystemTray, n *notifications.NotificationService) {
	for {
		req, _ := http.NewRequest("GET", s.base+"/api/events", nil)
		req.Header.Set("Authorization", "Bearer "+s.token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var event string
			sc := bufio.NewScanner(resp.Body)
			for sc.Scan() {
				line := sc.Text()
				if v, ok := strings.CutPrefix(line, "event: "); ok {
					event = v
					continue
				}
				data, ok := strings.CutPrefix(line, "data: ")
				if !ok || (event != "timer.tick" && event != "timer.phase") {
					continue
				}
				var e struct{ Data snapshot }
				if json.Unmarshal([]byte(data), &e) != nil {
					continue
				}
				t := e.Data
				if t.State == "idle" {
					tray.SetTooltip("Terran CC")
				} else {
					tray.SetTooltip(fmt.Sprintf("Terran CC · %s %02d:%02d%s", phaseNames[t.Phase], t.Remaining/60, t.Remaining%60,
						map[bool]string{true: " (paused)"}[t.State == "paused"]))
				}
				if event == "timer.phase" {
					body := "Time to focus."
					if t.Phase != "focus" {
						body = "Focus done. Take a " + strings.ToLower(phaseNames[t.Phase]) + "."
					}
					if err := n.SendNotification(notifications.NotificationOptions{
						ID: fmt.Sprintf("phase-%d", time.Now().Unix()), Title: "Terran CC", Body: body,
					}); err != nil {
						log.Printf("notification: %v", err)
					}
					s.flashAll()
				}
			}
			resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
}
