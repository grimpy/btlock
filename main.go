package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
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

func waitForChange(conn *dbus.Conn, obj dbus.BusObject) bool {
	obj.AddMatchSignal("org.freedesktop.DBus.Properties", "PropertiesChanged", dbus.WithMatchOption("arg0", "org.bluez.Device1"))

	c := make(chan *dbus.Signal, 10)
	conn.Signal(c)
	for v := range c {
		var changes map[string]dbus.Variant
		changes = v.Body[1].(map[string]dbus.Variant)
		if val, ok := changes["Connected"]; ok {
			isconnected := val.Value().(bool)
			log.Println("Isconnected", isconnected)
			obj.RemoveMatchSignal("org.freedesktop.DBus.Properties", "PropertiesChanged", dbus.WithMatchOption("arg0", "org.bluez.Device1"))
			//conn.RemoveMatchSignal(matches...)
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

func getDevice(conn *dbus.Conn, devicepath string) (dbus.BusObject, error) {
	obj := conn.Object("org.bluez", dbus.ObjectPath(devicepath))
	_, err := obj.GetProperty("org.bluez.Device1.Connected")
	return obj, err
}

func tryConnect(conn *dbus.Conn, obj dbus.BusObject) bool {
	var connected dbus.Variant
	connected, _ = obj.GetProperty("org.bluez.Device1.Connected")
	if !connected.Value().(bool) {
		log.Println("Not connected, trying to connect")
		obj.Call("org.bluez.Device1.Connect", dbus.FlagNoAutoStart)
		connected, _ = obj.GetProperty("org.bluez.Device1.Connected")
	}
	if connected.Value().(bool) {
		log.Println("Connected waiting for connection to terminate")
		waitForChange(conn, obj)
		return true
	}
	return false
}

func sendNotification(conn *dbus.Conn, message string, replaceId uint32, level byte) uint32 {
	iconName := "locked"
	n := notify.Notification{
		AppName:       "BT Autolocker",
		ReplacesID:    replaceId,
		AppIcon:       iconName,
		Summary:       "BT Autolocker",
		Body:          message,
		Actions:       []string{}, // tuples of (action_key, label)
		Hints:         map[string]dbus.Variant{"urgency": dbus.MakeVariant(level)},
		ExpireTimeout: int32(5000),
	}

	createdID, _ := notify.SendNotification(conn, n)
	return createdID
}

func main() {
	var maxidletimeflag int
	var maxidletime uint32
	var lockapp string
	var macaddr string
	var device dbus.BusObject
	replaceId := uint32(0)
	flag.IntVar(&maxidletimeflag, "idletime", 30, "Idle time before invoking lock")
	flag.StringVar(&lockapp, "lockapp", "i3lock", "Command to invoke to lock")
	flag.StringVar(&macaddr, "macaddr", "", "Macaddress of device to check connection")
	flag.Parse()
	maxidletime = uint32(maxidletimeflag * 1000)

	app, err := shlex.Split(lockapp)
	if err != nil {
		fmt.Println("Invalid lockapp")
		os.Exit(1)
	}

	macaddr = strings.ToUpper(macaddr)
	devicepath := fmt.Sprintf("/org/bluez/hci0/dev_%s", strings.ReplaceAll(macaddr, ":", "_"))
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

	if macaddr != "" {
		device, err = getDevice(conn, devicepath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Device %s is not know please pair it first\n", macaddr)
			os.Exit(1)
		}
	}

	replaceId = sendNotification(sessionbus, "Autolocker started", replaceId, byte(1))

	for {
		idletime, _ := getIdleTime(X, scr)
		if idletime < maxidletime {
			sleeptime := maxidletime - idletime
			log.Println("Sleeping for ", sleeptime/1000)
			time.Sleep(time.Duration(sleeptime) * time.Millisecond)
			continue
		}
		showwarning := true
		if macaddr != "" {
			showwarning = !tryConnect(conn, device)
		}
		if showwarning {
			replaceId = sendNotification(sessionbus, "Locking in 5 seconds", replaceId, byte(2))
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
