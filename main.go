package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type bytes []byte

type Conn struct {
	conn    interface{}
	scanner *bufio.Scanner
}
type Msg struct {
	Name string
	To   []string
	Str  string
	Time int64
}

func (c *Conn) Scan() (str string, err error) {
	str = ""
	err = nil
	switch t := c.conn.(type) {
	case net.Conn:
		if chk := c.scanner.Scan(); chk == false {
			str, err = "", errors.New("connection finished")
			return
		}
		str = string(c.scanner.Bytes())

	case *websocket.Conn:
		var b bytes
		_, b, err = t.ReadMessage()
		str = string(b)
		if err != nil {
			return
		}

	default:
		log.Panic("Conn is not net.Conn or websocket.Conn : " + reflect.TypeOf(c.conn).String())
	}

	return
}
func (c *Conn) Send(str string) error {
	switch t := c.conn.(type) {
	case net.Conn:
		_, err := fmt.Fprintf(t, "%s\n", str)
		if err != nil {
			return err
		}
	case *websocket.Conn:
		err := t.WriteMessage(1, bytes(str))
		if err != nil {
			return err
		}
	default:
		log.Panic("Conn is not net.Conn or websocket.Conn : " + reflect.TypeOf(c.conn).Elem().String())
	}
	return nil
}
func (c *Conn) Close() error {
	switch t := c.conn.(type) {
	case net.Conn:
		if err := t.Close(); err != nil {
			return err
		}
	case *websocket.Conn:
		if err := t.Close(); err != nil {
			return err
		}
	default:
		log.Panic("Conn is not net.Conn or websocket.Conn : " + reflect.TypeOf(c.conn).Elem().String())
	}
	return nil
}
func NewConn(conn interface{}) (c *Conn) {
	switch t := conn.(type) {
	case net.Conn:
		c = &Conn{
			conn:    conn,
			scanner: bufio.NewScanner(t),
		}
	case *websocket.Conn:
		c = &Conn{
			conn: conn,
		}
	}

	return
}

type User struct {
	IP      string
	Name    string
	conn    *Conn
	isAlive bool

	WriteCh chan Msg
	ReadCh  chan Msg
	Status  chan bool
}

func (usr *User) Listener() {

	defer close(usr.ReadCh)
	defer close(usr.WriteCh)
	defer usr.conn.Close()

	go func() {
		for {
			if usr.isAlive == false {
				return
			}
			select {
			case <-usr.Status:
				usr.isAlive = false
				return
			default:
			}
			str, err := usr.conn.Scan()
			if err != nil {
				close(usr.Status)
				return
			}

			var msg Msg
			err = json.Unmarshal(bytes(str), &msg)
			msg.Name = usr.Name
			msg.Time = time.Now().Unix()

			if err != nil {
				log.Printf("message is not json : %s\n", err)
				close(usr.Status)
				return
			}

			usr.ReadCh <- msg
		}
	}()
	for {
		if usr.isAlive == false {
			return
		}
		select {
		case <-usr.Status:
			usr.isAlive = false
			return
		case msg := <-usr.WriteCh:
			str, err := json.Marshal(msg)
			if err != nil {
				close(usr.Status)
				return
			}
			err = usr.conn.Send(string(str))
			if err != nil {
				close(usr.Status)
				return
			}
		default:
		}
	}
}
func NewUser(IP string, conn *Conn) *User {
	name := genName()
	usr := &User{
		IP,
		name,
		conn,
		true,
		make(chan Msg, 4),
		make(chan Msg, 4),
		make(chan bool),
	}

	b, e := json.Marshal(Msg{
		Name: "SYSTEM",
		Str:  "Hello, " + name + "!\n",
		Time: time.Now().Unix(),
	})
	if e != nil {
		log.Fatal(e)
	}

	conn.Send(string(b))

	go usr.Listener()

	return usr
}

var RecvMsgCh = make(chan Msg, 256)
var SendMsgCh = make(chan Msg, 256)
var UserCh = make(chan *User, 256)
var Users = make(map[string]*User)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func chkName(name string) bool {
	_, nok := Users[name]
	return !nok
}
func genName() string {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for {
		var b bytes
		arr := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

		for i := 0; i < 10; i++ {
			b = append(b, arr[r.Intn(len(arr))])
		}

		if chkName(string(b)) == true {
			return string(b)
		}
	}
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "chat.html")
	})
	r.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Panic(err)
		}
		UserCh <- NewUser(conn.RemoteAddr().String(), NewConn(conn))
		log.Printf("websocket user connected : %s\n", conn.RemoteAddr())
	})
	go func() {
		err := http.ListenAndServe(":8080", r)
		if err != nil {
			log.Panic(err)
		}
	}()

	go func() {
		listener, err := net.Listen("tcp", ":4040")
		if err != nil {
			log.Panic(err)
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Panic(err)
			}

			UserCh <- NewUser(conn.RemoteAddr().String(), NewConn(conn))
			log.Printf("tcp user connected : %s\n", conn.RemoteAddr())
		}
	}()

	log.Println("server start")

	for {
		for k, u := range Users {
			if u.isAlive == false {
				log.Println(u.Name, "is quit")

				delete(Users, k)
			}
		}

	NEWUSER:
		for {
			select {
			case u := <-UserCh:
				if _, ok := Users[u.Name]; ok != false {
					log.Fatal(u.Name, "is already login")
				}
				Users[u.Name] = u
			default:
				break NEWUSER
			}
		}

		for _, u := range Users {
		RECVMSG:
			for {
				select {
				case msg := <-u.ReadCh:
					RecvMsgCh <- msg
				default:
					break RECVMSG
				}
			}
		}
	WRITEMSG:
		for {
			select {
			case msg := <-RecvMsgCh:
				if msg.To == nil || len(msg.To) == 0 {
					for _, u := range Users {
						u.WriteCh <- msg
					}
				} else {
					for _, name := range msg.To {
						Users[name].WriteCh <- msg
					}
				}
			default:
				break WRITEMSG
			}
		}
	}
}
