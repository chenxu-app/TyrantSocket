package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bipabo1l/TyrantSocket/protocol"
)

//记录所有Agent
var clientArr = make(map[string]net.Conn)

type AgentMsg struct {
	Session int64
	IP      string
	Message string
	Status  string
}

//定义CheckError方法，避免写太多到 if err!=nil
func CheckError(err error) {

	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error:%s", err.Error())

		os.Exit(1)
	}

}

//自定义log
func Log(v ...interface{}) {

	log.Println(v...)
}

var ch1 = make(chan int, 1)

func main() {

	//server_listener, err := net.Listen("tcp", "192.168.0.8:8848")
	server_listener, err := net.Listen("tcp", "127.0.0.1:8848")

	CheckError(err)

	defer server_listener.Close()

	Log("Waiting for clients connect")

	go getConn()

	for {
		new_conn, err := server_listener.Accept()

		clientIPList := strings.Split(new_conn.RemoteAddr().String(), ":")

		clientIP := new_conn.RemoteAddr().String()
		if len(clientIPList) > 0 {
			clientIP = clientIPList[0]
		}
		log.Println(clientIP)
		clientArr[clientIP] = new_conn

		CheckError(err)

		log.Println(new_conn.RemoteAddr().String() + " 上线了")

		go ServerMsgHandler(new_conn)
	}

}

func getConn() {
	http.HandleFunc("/", sayhelloName)
	err := http.ListenAndServe(":8849", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

type IPList struct {
	IPPort   string   `json:"ipport"`
	ConnList net.Conn `json:"netconn"`
}

type IPListMap struct {
	IPListMap []IPList
}

func sayhelloName(w http.ResponseWriter, r *http.Request) {

	r.ParseForm()
	key := ""
	value := ""
	for k, v := range r.Form {
		if k != "" && strings.Join(v, "") != "" {
			key = k
			value = strings.Join(v, "")
		}
	}

	//监听8849端口判断批量getStatus动作
	if key == "key" && value == "getstatus" {
		data, err := json.Marshal(clientArr)
		if err != nil {
			log.Println(err)
			return
		}
		s := strings.Split(string(data[:]), ",")
		ll := ""
		if len(s) > 0 {
			for _, w := range s {
				k := strings.Split(w, ":")
				if len(k) > 0 && ll != "" {
					ll = ll + "," + k[0]
					ll = strings.Replace(ll, "{", "", -1)
					ll = strings.Replace(ll, `"`, "", -1)
					ll = strings.Replace(ll, `}`, "", -1)
				} else if len(k) > 0 {
					ll = k[0]
					ll = strings.Replace(ll, "{", "", -1)
					ll = strings.Replace(ll, `"`, "", -1)
					ll = strings.Replace(ll, `}`, "", -1)
				}
			}
		}

		fmt.Fprintf(w, `{"Agent":"`+ll+`"}`)
	}

	//令agent停止命令
	if key == "key" && protocol.Substr2(value, 0, 4) == "stop" {

		ip := protocol.Substr2(value, 4, len(value))

		log.Println("-----------------------------")
		log.Println(clientArr)

		findConn := clientArr[ip]

		go StopClient(findConn)

		fmt.Fprintf(w, "1")
	}

	//令agent开启命令
	if key == "key" && protocol.Substr2(value, 0, 5) == "start" {

		ip := protocol.Substr2(value, 5, len(value))

		log.Println("-----------------------------")
		log.Println(clientArr)

		findConn := clientArr[ip]

		go BeginClient(findConn)

		fmt.Fprintf(w, "1")
	}
}

//服务端消息处理
func ServerMsgHandler(conn net.Conn) {

	//存储被截断的数据
	tmpbuf := make([]byte, 0)
	buf := make([]byte, 1024)

	defer conn.Close()

	//接收解包
	readchan := make(chan []byte, 16)
	go ReadChan(readchan)

	for {
		//读取客户端发来的消息
		n, err := conn.Read(buf)

		if err != nil {

			fmt.Println("connection close")
			removeClient(conn)
			conn.Close()
			return
		}

		//解包
		tmpbuf = protocol.Depack(append(tmpbuf, buf[:n]...))
		//tmpbuf = buf

		fmt.Println("client say:", string(tmpbuf))

		//判断解析json
		var agentmsg AgentMsg
		json.Unmarshal([]byte(string(tmpbuf)), &agentmsg)
		Msg := tmpbuf
		log.Println(agentmsg.Status)

		//向客户端发送消息
		go WriteMsgToClient(conn)

		go func() {
			if <-ch1 == 2 {
				WriteMsgToClient2(conn)
			}
		}()



		beatch := make(chan byte)
		//心跳计时，默认30秒
		go HeartBeat(conn, beatch, 30)
		//检测每次Client是否有数据传来
		go HeartChanHandler(Msg, beatch)

	}

}

//处理心跳,根据HeartChanHandler判断Client是否在设定时间内发来信息
func HeartBeat(conn net.Conn, heartChan chan byte, timeout int) {
	select {
	case hc := <-heartChan:
		Log("<-heartChan:", string(hc))
		conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		break
	case <-time.After(time.Second * 30):
		Log("timeout")
		conn.Close()
	}
}

//服务端向客户端发送消息
func WriteMsgToClient(conn net.Conn) {
	log.Println(conn.RemoteAddr())
	talk := "RUN"
	smsg := protocol.Enpack([]byte(talk))
	conn.Write(smsg)
}

func WriteMsgToClient2(conn net.Conn) {
	talk := "GETSTATUS"
	smsg := protocol.Enpack([]byte(talk))
	conn.Write(smsg)
}

//Server表示不想跟您通信咯
func StopClient(conn net.Conn) {
	talk := "STOP"
	smsg := protocol.Enpack([]byte(talk))
	conn.Write(smsg)
}

//Server表示不想跟您通信咯
func BeginClient(conn net.Conn) {
	talk := "BEGIN"
	smsg := protocol.Enpack([]byte(talk))
	conn.Write(smsg)
}

//处理心跳channel
func HeartChanHandler(n []byte, beatch chan byte) {
	for _, v := range n {
		beatch <- v
		log.Println(v)
	}
	close(beatch)
}

//从channell中读取数据
func ReadChan(readchan chan []byte) {

	for {
		select {
		case data := <-readchan:
			Log(string(data))
		}
	}
}

func removeClient(new_conn net.Conn) {
	log.Println(clientArr)
	log.Println(new_conn.RemoteAddr().String() + " 已经阵亡")
	delete(clientArr, new_conn.RemoteAddr().String())
	log.Println("delete close conn")
	log.Println(clientArr)
	return
}
