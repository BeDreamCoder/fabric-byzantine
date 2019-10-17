package main

import (
	"encoding/json"
	"errors"
	"fabric-byzantine/server"
	"fabric-byzantine/server/mysql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

func timerTask() {
	c := time.Tick(5 * time.Second)
	for {
		<-c
		result, _ := server.GetSdkProvider().QueryCC("mychannel1", "token",
			"getPeers", [][]byte{[]byte("fab")})
		peers := make(map[string]bool)
		json.Unmarshal(result, &peers)
		index := 0
		for _, v := range peers {
			if v {
				break
			}
			index++
		}
		if index >= 10 {
			index = 0
		}
		go server.GetSdkProvider().InvokeCC(index, "mychannel1", "token", "transfer",
			[][]byte{[]byte("fab"), []byte("alice"), []byte("bob"), []byte("1"), []byte("true")})
	}
}

var addr = flag.String("addr", ":8080", "http service address")

var upgrader = websocket.Upgrader{} // use default options

func getBalance(w http.ResponseWriter, r *http.Request) {
	user := r.FormValue("user")
	data, err := server.GetSdkProvider().QueryCC("mychannel1", "token",
		"balance", [][]byte{[]byte("fab"), []byte(user)})
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
	} else {
		w.WriteHeader(200)
		w.Write(data)
	}
}

func block(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	uid := uuid.NewV4().String()
	fmt.Println(uid)
	ch := make(chan []byte)
	server.BlockChans.Store(uid, ch)

	for {
		//mt, message, err := c.ReadMessage()
		//if err != nil {
		//	log.Println("read:", err)
		//	return
		//}
		//log.Printf("msg type: %d, recv: %s", mt, message)
		select {
		case datas := <-ch:
			log.Println("block ws response:", string(datas))
			err = c.WriteMessage(websocket.TextMessage, datas)
			if err != nil {
				log.Println("block ws response err:", err)
				server.BlockChans.Delete(uid)
				return
			}
		}
	}
}

func blockPage(w http.ResponseWriter, r *http.Request) {
	var err error
	var datas []byte
	defer func() {
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(200)
			w.Write(datas)
		}
	}()
	var pageId, size int
	pageId, err = strconv.Atoi(r.FormValue("id"))
	if err != nil {
		return
	}
	size, err = strconv.Atoi(r.FormValue("size"))
	if err != nil {
		return
	}
	if pageId < 1 || size < 1 {
		err = errors.New("invalid pageId")
		return
	}
	datas, err = mysql.BlockPage(pageId, size)
}

func transaction(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	uid := uuid.NewV4().String()
	fmt.Println(uid)
	ch := make(chan []byte)
	server.TxChans.Store(uid, ch)

	for {
		select {
		case datas := <-ch:
			log.Println("transaction ws response:", string(datas))
			err = c.WriteMessage(websocket.TextMessage, datas)
			if err != nil {
				log.Println("transaction ws response err:", err)
				server.TxChans.Delete(uid)
				return
			}
		}
	}
}

func transactionPage(w http.ResponseWriter, r *http.Request) {
	var err error
	var datas []byte
	defer func() {
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(200)
			w.Write(datas)
		}
	}()
	var pageId, size int
	pageId, err = strconv.Atoi(r.FormValue("id"))
	if err != nil {
		return
	}
	size, err = strconv.Atoi(r.FormValue("size"))
	if err != nil {
		return
	}
	if pageId < 1 || size < 1 {
		err = errors.New("invalid pageId")
		return
	}
	datas, err = mysql.TransactionPage(pageId, size)
}

func main() {
	defer mysql.CloseDB()

	go server.GetSdkProvider().BlockListener("mychannel1")
	go timerTask()

	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/balance", getBalance)
	http.HandleFunc("/block/page", blockPage)
	http.HandleFunc("/block", block)
	http.HandleFunc("/transaction", transaction)
	http.HandleFunc("/transaction/page", transactionPage)
	http.HandleFunc("/", home)
	http.HandleFunc("/echo", echo)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	homeTemplate.Execute(w, "ws://"+r.Host+"/echo")
}

func echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("msg type: %d, recv: %s", mt, message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<script>  
window.addEventListener("load", function(evt) {

    var output = document.getElementById("output");
    var input = document.getElementById("input");
    var ws;

    var print = function(message) {
        var d = document.createElement("div");
        d.innerHTML = message;
        output.appendChild(d);
    };

    document.getElementById("open").onclick = function(evt) {
        if (ws) {
            return false;
        }
        ws = new WebSocket("{{.}}");
        ws.onopen = function(evt) {
            print("OPEN");
        }
        ws.onclose = function(evt) {
            print("CLOSE");
            ws = null;
        }
        ws.onmessage = function(evt) {
            print("RESPONSE: " + evt.data);
        }
        ws.onerror = function(evt) {
            print("ERROR: " + evt.data);
        }
        return false;
    };

    document.getElementById("send").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        print("SEND: " + input.value);
        ws.send(input.value);
        return false;
    };

    document.getElementById("close").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        ws.close();
        return false;
    };

});
</script>
</head>
<body>
<table>
<tr><td valign="top" width="50%">
<p>Click "Open" to create a connection to the server, 
"Send" to send a message to the server and "Close" to close the connection. 
You can change the message and send multiple times.
<p>
<form>
<button id="open">Open</button>
<button id="close">Close</button>
<p><input id="input" type="text" value="Hello world!">
<button id="send">Send</button>
</form>
</td><td valign="top" width="50%">
<div id="output"></div>
</td></tr></table>
</body>
</html>
`))
