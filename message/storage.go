// Copyright © 2014 Terry Mao, LiuDing All rights reserved.
// This file is part of gopush-cluster.

// gopush-cluster is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// gopush-cluster is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with gopush-cluster.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	log "github.com/alecthomas/log4go"
	"encoding/json"
	"errors"
	"github.com/loyalpartner/gopush-cluster/rpc"
)

const (
	RedisStorageType = "redis"
	MySQLStorageType = "mysql"
	ketamaBase       = 255
	saveBatchNum     = 1000
)

var (
	UseStorage     Storage
	ErrStorageType = errors.New("unknown storage type")
)

// Stored messages interface
type Storage interface {
	// GetPrivate get private msgs.
	GetPrivate(key string, mid int64) ([]*rpc.Message, error)
	// SavePrivate Save single private msg.
	SavePrivate(key string, msg json.RawMessage, mid int64, expire uint) error
	// Save private msgs return failed keys.
	SavePrivates(keys []string, msg json.RawMessage, mid int64, expire uint) ([]string, error)
	// DelPrivate delete private msgs.
	DelPrivate(key string) error
}

// InitStorage init the storage type(mysql or redis).
func InitStorage() error {
	if Conf.StorageType == RedisStorageType {
		UseStorage = NewRedisStorage()
	} else if Conf.StorageType == MySQLStorageType {
		UseStorage = NewMySQLStorage()
	} else {
		log.Error("unknown storage type: \"%s\"", Conf.StorageType)
		return ErrStorageType
	}
	return nil
}
