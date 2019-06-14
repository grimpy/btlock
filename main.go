package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/screensaver"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/esiqveland/notify"
	"github.com/godbus/dbus"
	"github.com/google/shlex"
)

func getIdleTime(con *xgb.Conn, drawable xproto.Drawable) (uint32, error) {

	request := screensaver.QueryInfo(con, drawable)
	qinfo, err := request.Reply()
	if err != nil {
		return 0, err
	}
	return qinfo.MsSinceUserInput, nil
}

func isIdle(con *xgb.Conn, drawable xproto.Drawable, maxidletime uint32) bool {
	idletime, _ := getIdleTime(con, drawable)
	log.Println("Idle time", idletime)
	return idletime > maxidletime
}

func waitForChange(conn *dbus.Conn, devicepath string) bool {
	matches := []dbus.MatchOption{dbus.WithMatchOption("path", devicepath),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchOption("arg0", "org.bluez.Device1")}
	conn.AddMatchSignal(matches...)

	c := make(chan *dbus.Signal, 10)
	conn.Signal(c)
	for v := range c {
		var changes map[string]dbus.Variant
		changes = v.Body[1].(map[string]dbus.Variant)
		if val, ok := changes["Connected"]; ok {
			isconnected := val.Value().(bool)
			log.Println("Isconnected", isconnected)
			conn.RemoveMatchSignal(matches...)
			return isconnected
		}
	}
	return false
}

func lock(app []string) {
	cmd := exec.Command(app[0], app[1:]...)
	err := cmd.Run()
	log.Println(err)
}

func tryConnect(conn *dbus.Conn, devicepath string) bool {
	var connected dbus.Variant
	obj := conn.Object("org.bluez", dbus.ObjectPath(devicepath))
	connected, _ = obj.GetProperty("org.bluez.Device1.Connected")
	if !connected.Value().(bool) {
		log.Println("Not connected, trying to connect")
		obj.Call("org.bluez.Device1.Connect", dbus.FlagNoAutoStart)
		connected, _ = obj.GetProperty("org.bluez.Device1.Connected")
	}
	if connected.Value().(bool) {
		log.Println("Connected waiting for connection to terminate")
		waitForChange(conn, devicepath)
		return true
	}
	return false
}

func sendNotification(conn *dbus.Conn, message string, replaceId uint32) uint32 {
	iconName := "mail-unread"
	n := notify.Notification{
		AppName:       "BT Autolocker",
		ReplacesID:    replaceId,
		AppIcon:       iconName,
		Summary:       "BT Autolocker",
		Body:          message,
		Actions:       []string{}, // tuples of (action_key, label)
		Hints:         map[string]dbus.Variant{},
		ExpireTimeout: int32(5000),
	}

	// Ship it!
	createdID, _ := notify.SendNotification(conn, n)
	return createdID
}

func main() {
	var maxidletimeflag int
	var maxidletime uint32
	var lockapp string
	replaceId := uint32(0)
	flag.IntVar(&maxidletimeflag, "idletime", 30, "Idle time before invoking lock")
	flag.StringVar(&lockapp, "lockapp", "i3lock", "Command to invoe to lock")
	flag.Parse()
	maxidletime = uint32(maxidletimeflag * 1000)

	app, err := shlex.Split(lockapp)
	if err != nil {
		fmt.Println("Invalid lockapp")
		os.Exit(1)
	}

	devicepath := "/org/bluez/hci0/dev_A0_9E_1A_14_FE_10"
	X, err := xgb.NewConn()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(X.DisplayNumber)
	scr := xproto.Drawable(xproto.Setup(X).DefaultScreen(X).Root)
	screensaver.Init(X)

	conn, err := dbus.SystemBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", err)
		os.Exit(1)
	}
	sessionbus, err := dbus.SessionBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to session bus:", err)
		os.Exit(1)
	}

	sendNotification(sessionbus, "Autolocker started", replaceId)

	for {
		idletime, _ := getIdleTime(X, scr)
		if idletime < maxidletime {
			sleeptime := maxidletime - idletime
			log.Println("Sleeping for ", sleeptime/1000)
			time.Sleep(time.Duration(sleeptime) * time.Millisecond)
			continue
		}
		if !tryConnect(conn, devicepath) {
			sendNotification(sessionbus, "Locking in 5 seconds", replaceId)
			time.Sleep(time.Duration(5) * time.Second)
			if isIdle(X, scr, maxidletime) {
				log.Println("Locking now")
				lock(app)
				time.Sleep(time.Duration(5) * time.Second)
			}
		} else {
			lock(app)

		}
	}

}
