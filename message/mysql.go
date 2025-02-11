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
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/loyalpartner/gopush-cluster/ketama"
	myrpc "github.com/loyalpartner/gopush-cluster/rpc"
	_ "github.com/go-sql-driver/mysql"
	"strconv"
	"strings"
	"time"
)

const (
	savePrivateMsgSQL = "INSERT INTO private_msg(skey,mid,ttl,msg,ctime,mtime) VALUES(?,?,?,?,?,?)"
	// TODO limit
	getPrivateMsgSQL        = "SELECT mid, ttl, msg FROM private_msg WHERE skey=? AND mid>? ORDER BY mid"
	delExpiredPrivateMsgSQL = "DELETE FROM private_msg WHERE ttl<=?"
	delPrivateMsgSQL        = "DELETE FROM private_msg WHERE skey=?"
)

var (
	ErrNoMySQLConn     = errors.New("can't get a mysql db")
	mysqlSourceSpliter = ":"
)

// MySQL Storage struct
type MySQLStorage struct {
	pool map[string]*sql.DB
	ring *ketama.HashRing
}

// NewMySQLStorage initialize mysql pool and consistency hash ring.
func NewMySQLStorage() *MySQLStorage {
	dbPool := make(map[string]*sql.DB)
	ring := ketama.NewRing(ketamaBase)
	for n, source := range Conf.MySQLSource {
		nw := strings.Split(n, mysqlSourceSpliter)
		if len(nw) != 2 {
			err := errors.New("node config error, it's nodeN:W")
			log.Error("strings.Split(\"%s\", :) failed (%v)", n, err)
			panic(err)
		}
		w, err := strconv.Atoi(nw[1])
		if err != nil {
			log.Error("strconv.Atoi(\"%s\") failed (%v)", nw[1], err)
			panic(err)
		}
		db, err := sql.Open("mysql", source)
		if err != nil {
			log.Error("sql.Open(\"mysql\", %s) failed (%v)", source, err)
			panic(err)
		}
		dbPool[nw[0]] = db
		ring.AddNode(nw[0], w)
	}
	ring.Bake()
	s := &MySQLStorage{pool: dbPool, ring: ring}
	go s.clean()
	return s
}

// SavePrivate implements the Storage SavePrivate method.
func (s *MySQLStorage) SavePrivate(key string, msg json.RawMessage, mid int64, expire uint) error {
	db := s.getConn(key)
	if db == nil {
		return ErrNoMySQLConn
	}
	now := time.Now()
	_, err := db.Exec(savePrivateMsgSQL, key, mid, now.Unix()+int64(expire), []byte(msg), now, now)
	if err != nil {
		log.Error("db.Exec(\"%s\",\"%s\",%d,%d,%d,\"%s\",now,now) failed (%v)", savePrivateMsgSQL, key, mid, expire, string(msg), err)
		return err
	}
	return nil
}

// SavePrivates implements the Storage SavePrivates method.
func (s *MySQLStorage) SavePrivates(keys []string, msg json.RawMessage, mid int64, expire uint) ([]string, error) {
	// TODO
	return nil, nil
}

// GetPrivate implements the Storage GetPrivate method.
func (s *MySQLStorage) GetPrivate(key string, mid int64) ([]*myrpc.Message, error) {
	db := s.getConn(key)
	if db == nil {
		return nil, ErrNoMySQLConn
	}
	now := time.Now().Unix()
	rows, err := db.Query(getPrivateMsgSQL, key, mid)
	if err != nil {
		log.Error("db.Query(\"%s\",\"%s\",%d,now) failed (%v)", getPrivateMsgSQL, key, mid, err)
		return nil, err
	}
	msgs := []*myrpc.Message{}
	for rows.Next() {
		expire := int64(0)
		cmid := int64(0)
		msg := []byte{}
		if err := rows.Scan(&cmid, &expire, &msg); err != nil {
			log.Error("rows.Scan() failed (%v)", err)
			return nil, err
		}
		if now > expire {
			log.Warn("user_key: \"%s\" mid: %d expired", key, cmid)
			continue
		}
		msgs = append(msgs, &myrpc.Message{MsgId: cmid, GroupId: myrpc.PrivateGroupId, Msg: json.RawMessage(msg)})
	}
	return msgs, nil
}

// DelPrivate implements the Storage DelPrivate method.
func (s *MySQLStorage) DelPrivate(key string) error {
	db := s.getConn(key)
	if db == nil {
		return ErrNoMySQLConn
	}
	res, err := db.Exec(delPrivateMsgSQL, key)
	if err != nil {
		log.Error("db.Exec(\"%s\", \"%s\") error(%v)", delPrivateMsgSQL, key, err)
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Error("res.RowsAffected() error(%v)", err)
		return err
	}
	log.Info("user_key: \"%s\" clean message num: %d", rows)
	return nil
}

// clean delete expired messages peroridly.
func (s *MySQLStorage) clean() {
	for {
		log.Info("clean mysql expired message start")
		now := time.Now().Unix()
		affect := int64(0)
		for _, db := range s.pool {
			res, err := db.Exec(delExpiredPrivateMsgSQL, now)
			if err != nil {
				log.Error("db.Exec(\"%s\", %d) failed (%v)", delExpiredPrivateMsgSQL, now, err)
				continue
			}
			aff, err := res.RowsAffected()
			if err != nil {
				log.Error("res.RowsAffected() error(%v)", err)
				continue
			}
			affect += aff
		}
		log.Info("clean mysql expired message finish, num: %d", affect)
		time.Sleep(Conf.MySQLClean)
	}
}

// getConn get the connection of matching with key using ketama hash
func (s *MySQLStorage) getConn(key string) *sql.DB {
	if len(s.pool) == 0 {
		return nil
	}
	node := s.ring.Hash(key)
	p, ok := s.pool[node]
	if !ok {
		log.Warn("user_key: \"%s\" hit mysql node: \"%s\" not in pool", key, node)
		return nil
	}
	log.Info("user_key: \"%s\" hit mysql node: \"%s\"", key, node)
	return p
}
