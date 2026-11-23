package storage

import (
	"encoding/json"
	"errors"
	"time"

	bbolt "go.etcd.io/bbolt"
)

var (
	ErrSyncDeviceMissing = errors.New("sync device not found")
	ErrSyncDeviceLimit = errors.New("sync device limit reached")
)

var syncDeviceBucket = []byte("drive_sync_devices_v1")

type SyncDevice struct {
 UploadGranted bool
 TreeGranted bool
 ReceiveGranted bool
	ID string
	UserID uint
	Name string
	Root string
	Digest string
	Binding string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type SyncDeviceStore struct {
	db *bbolt.DB
}

func NewSyncDeviceStore(db *bbolt.DB) *SyncDeviceStore {
	return &SyncDeviceStore{db: db}
}

func (s *SyncDeviceStore) Create(device SyncDevice) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(syncDeviceBucket)
		if err != nil {
			return err
		}
		if b.Get([]byte(device.ID)) != nil {
			return errors.New("duplicate sync device")
		}
		owned, total := 0, 0
		now := time.Now()
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var existing SyncDevice
			if err := json.Unmarshal(v, &existing); err != nil {
				return err
			}
			if !now.Before(existing.ExpiresAt) {
				if err := c.Delete(); err != nil {
					return err
				}
				continue
			}
			total++
			if existing.UserID == device.UserID {
				owned++
			}
		}
		if owned >= 20 || total >= 4096 {
			return ErrSyncDeviceLimit
		}
		raw, err := json.Marshal(device)
		if err != nil {
			return err
		}
		return b.Put([]byte(device.ID), raw)
	})
}

func (s *SyncDeviceStore) Get(id string) (SyncDevice, error) {
	var device SyncDevice
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(syncDeviceBucket)
		if b == nil {
			return ErrSyncDeviceMissing
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return ErrSyncDeviceMissing
		}
		return json.Unmarshal(raw, &device)
	})
	return device, err
}

func (s *SyncDeviceStore) List(userID uint) ([]SyncDevice, error) {
	devices := make([]SyncDevice, 0)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(syncDeviceBucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, raw []byte) error {
			var device SyncDevice
			if err := json.Unmarshal(raw, &device); err != nil {
				return err
			}
			if device.UserID == userID {
				devices = append(devices, device)
			}
			return nil
		})
	})
	return devices, err
}

func (s *SyncDeviceStore) Revoke(userID uint, id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(syncDeviceBucket)
		if b == nil {
			return ErrSyncDeviceMissing
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return ErrSyncDeviceMissing
		}
		var device SyncDevice
		if err := json.Unmarshal(raw, &device); err != nil {
			return err
		}
		if device.UserID != userID {
			return ErrSyncDeviceMissing
		}
		return b.Delete([]byte(id))
	})
}
