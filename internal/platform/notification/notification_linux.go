//go:build linux && !android

package notification

import (
	"fmt"
	"github.com/godbus/dbus/v5"
	branding "reia-clash/ico"
	"sync"
)

type Notifier struct {
	conn    *dbus.Conn
	signals chan *dbus.Signal
	done    chan struct{}
	mu      sync.Mutex
	ids     map[uint32]bool
	lastID  uint32
	open    func()
	show    func(string, string)
}

func New(show func(string, string), open func()) (*Notifier, error) {
	notifier := &Notifier{done: make(chan struct{}), ids: map[uint32]bool{}, open: open, show: show}
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return notifier, err
	}
	notifier.conn = connection
	if err := connection.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.Notifications"), dbus.WithMatchMember("ActionInvoked")); err != nil {
		connection.Close()
		notifier.conn = nil
		return notifier, err
	}
	notifier.signals = make(chan *dbus.Signal, 16)
	connection.Signal(notifier.signals)
	go func() {
		for {
			select {
			case <-notifier.done:
				return
			case signal, ok := <-notifier.signals:
				if !ok {
					return
				}
				if signal == nil || len(signal.Body) < 2 {
					continue
				}
				id, ok := signal.Body[0].(uint32)
				action, _ := signal.Body[1].(string)
				notifier.mu.Lock()
				ours := ok && notifier.ids[id]
				notifier.mu.Unlock()
				if ours && (action == "default" || action == "open") {
					go open()
				}
			}
		}
	}()
	return notifier, nil
}
func (n *Notifier) Show(title, message string) error {
	if n.conn == nil {
		n.show(title, message)
		return fmt.Errorf("系统通知会话不可用")
	}
	var id uint32
	n.mu.Lock()
	replaceID := n.lastID
	n.mu.Unlock()
	icon, _ := branding.DesktopIconPath()
	err := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").Call("org.freedesktop.Notifications.Notify", 0, branding.AppName, replaceID, icon, title, message, []string{"default", "打开面板", "open", "打开面板"}, map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant("reia")}, int32(-1)).Store(&id)
	if err != nil {
		n.show(title, message)
		return err
	}
	n.mu.Lock()
	n.ids = map[uint32]bool{id: true}
	n.lastID = id
	n.mu.Unlock()
	return nil
}
func (n *Notifier) Close() {
	close(n.done)
	if n.conn != nil {
		n.conn.Close()
	}
}

func RegisterApp() error { return nil }
