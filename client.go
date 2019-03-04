package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

type Msg struct {
	Name string
	To   []string
	Str  string
	Time int64
}

var scanner = bufio.NewScanner(os.Stdin)
var netScanner *bufio.Scanner

func main() {
	conn, err := net.Dial("tcp", "localhost:4040")
	if err != nil {
		log.Panic(err)
	}
	defer conn.Close()

	netScanner = bufio.NewScanner(conn)
	go func() {
		for {
			if chk := netScanner.Scan(); chk == false {
				log.Fatal("connection end")
				return
			}
			var msg Msg
			str := netScanner.Bytes()
			json.Unmarshal(str, &msg)

			fmt.Printf("%s %s : %s\n", time.Unix(msg.Time, 0), msg.Name, msg.Str)
		}
	}()
	for {
		scanner.Scan()
		str := scanner.Bytes()

		msg := Msg{"", []string{}, string(str), 0}
		m, err := json.Marshal(msg)
		if err != nil {
			log.Println(err)
		} else {
			fmt.Fprintf(conn, "%s\n", m)
		}
	}
}
