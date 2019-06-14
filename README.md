# Lock computer when idle or bluetooth devices disconnects

Have annoying colleages who can't help it from messing with your computer? Tend to forget to lock you computer? NO MORE!

This application will automaticly lock your computer when it looses connection with a bluetooth device (in my case my smartwatch) or when you computer is idle of certain amount of time.

This application works only on Linux in combination with Xorg uses bluez dbus interface and Xorg proto

## Install

`go install github.com/grimpy/btlock`

## Usage:

```
Usage of btlock:
  -idletime int
        Idle time before invoking lock (default 30)
  -lockapp string
        Command to invoe to lock (default "i3lock")
```
