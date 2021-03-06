package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jujili/exch"
	"github.com/jujili/exch/backtest"
	"github.com/jujili/jili/pkg/tools"
	"github.com/mattn/go-sqlite3"
)

// openToMemory 把 srcName 完整地拷贝到另一个内存数据库中，并返回内存数据库，
// 所以，对返回数据库的修改，并不会保存到 srcName 中。
func openToMemory(srcName string) *sql.DB {
	sqlite3conn := make([]*sqlite3.SQLiteConn, 0, 2)
	// fmt.Println(cap(sqlite3conn))
	sql.Register("sqlite3_with_hook_example",
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				sqlite3conn = append(sqlite3conn, conn)
				return nil
			},
		})

	srcDb, err := sql.Open("sqlite3_with_hook_example", srcName)
	if err != nil {
		log.Fatal(err)
	}
	defer srcDb.Close()
	srcDb.Ping()

	destDb, err := sql.Open("sqlite3_with_hook_example", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	// do NOT close destDB
	destDb.Ping()

	src, dest := sqlite3conn[0], sqlite3conn[1]

	copyDB(dest, src)

	return destDb
}

func copyDB(dst, src *sqlite3.SQLiteConn) {
	backup, err := dst.Backup("main", src, "main")
	if err != nil {
		return
	}
	defer backup.Finish()
	backup.Step(-1)
}

func tickPublishService(ctx context.Context, pub backtest.Publisher, db *sql.DB) {
	// 1514736000000 2018-01-01 0:00:00
	beginUTCMillisecond := int64(1514736000000)
	// 1577808000000 2020-01-01 0:00:00
	endUTCMillisecond := int64(1577808000000)
	// 1517414400000 2018-02-01 0:00:00
	// endUTCMillisecond := int64(1517414400000)
	// 1502942432285 2017/8/17 12:0:32 binance.sqlite3 中 btcusdt 的起时间
	// beginUTCMillisecond := int64(1502942432285)
	// endUTCMillisecond :=   int64(1502943432285)
	// 1509711755324 is 2017/11/3 20:22:35
	// endUTCMillisecond := int64(1509711755324)
	//
	beginTime := tools.LocalTime(beginUTCMillisecond)
	endTime := tools.LocalTime(endUTCMillisecond)
	log.Printf("数据起止时间为 [%s, %s)", beginTime, endTime)
	sql := fmt.Sprintf("SELECT id, price, quantity, utc FROM btcusdt WHERE utc BETWEEN %d AND %d", beginUTCMillisecond, endUTCMillisecond)
	rows, err := db.Query(sql)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	enc := exch.EncFunc()
	//
	for rows.Next() {
		var id int64
		var price, quantity float64
		var utc int64
		err = rows.Scan(&id, &price, &quantity, &utc)
		if err != nil {
			log.Fatal(err)
		}
		// log.Println(id, price, quantity, utc)
		tick := exch.NewTick(id, tools.LocalTime(utc), price, quantity)
		payload := enc(tick)
		// log.Println("src", tick)
		msg := message.NewMessage(watermill.NewUUID(), payload)
		if err := pub.Publish("tick", msg); err != nil {
			panic(err)
		}
		// log.Println("\t\t waiting next tick")
		// time.Sleep(time.Millisecond * 1)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	pub.Close()
	log.Println("pub.Close...")
}
