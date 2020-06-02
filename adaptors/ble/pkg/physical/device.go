package physical

import (
	"sync"
	"time"

	"github.com/bettercap/gatt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rancher/octopus/adaptors/ble/api/v1alpha1"
)

type Device interface {
	Configure(spec v1alpha1.BluetoothDeviceSpec, status v1alpha1.BluetoothDeviceStatus)
	Shutdown()
}
type device struct {
	sync.Mutex

	stop chan struct{}

	log     logr.Logger
	name    types.NamespacedName
	handler DataHandler

	syncInterval time.Duration
	timeout      time.Duration
	gattDevice   gatt.Device
	properties   []v1alpha1.DeviceProperty
	status       v1alpha1.BluetoothDeviceStatus
}

func NewDevice(log logr.Logger, name types.NamespacedName, handler DataHandler, param Parameters,
	gattDevice gatt.Device) Device {
	return &device{
		log:          log,
		name:         name,
		handler:      handler,
		syncInterval: param.SyncInterval,
		timeout:      param.Timeout,
		gattDevice:   gattDevice,
	}
}

func (d *device) Configure(spec v1alpha1.BluetoothDeviceSpec, status v1alpha1.BluetoothDeviceStatus) {
	d.connect(spec, status)
}

func (d *device) connect(spec v1alpha1.BluetoothDeviceSpec, status v1alpha1.BluetoothDeviceStatus) {
	if d.stop != nil {
		close(d.stop)
	}
	d.stop = make(chan struct{})

	var ticker = time.NewTicker(d.syncInterval * time.Second)
	defer ticker.Stop()
	d.log.Info("Sync interval is set to", d.syncInterval.String())

	// run periodically to sync device status
	for {
		cont := Controller{
			spec:   spec,
			status: status,
			done:   make(chan struct{}),
			log:    d.log,
		}
		// Register BLE device handlers.
		go d.gattDevice.Handle(
			gatt.PeripheralDiscovered(cont.onPeripheralDiscovered),
			gatt.PeripheralConnected(cont.onPeripheralConnected),
			gatt.PeripheralDisconnected(cont.onPeriphDisconnected),
		)

		d.gattDevice.Init(cont.onStateChanged)
		d.log.Info("Device Done")

		d.handler(d.name, cont.status)
		d.log.Info("Synced ble device status", cont.status)
		<-cont.done

		select {
		case <-d.stop:
			return
		case <-ticker.C:
		}
	}
}

func (d *device) Shutdown() {
	if d.stop != nil {
		close(d.stop)
	}
	d.log.Info("closed connection")
}
