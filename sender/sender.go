package main

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

const (
	receiverListenAddr = "127.0.0.1:8080"
	senderListenAddr   = "127.0.0.1:20002"
)

type Header struct {
	srcIP   uint32
	srcPort uint16
	seqNum  uint32
	ackNum  uint32
	flags   uint16
	winSize uint16
}

func sendUDP(packet []byte) {
	conn, err := net.Dial("udp", receiverListenAddr)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer conn.Close()

	conn.Write(packet)
}

func recvUDP() {
	pc, err := net.ListenPacket("udp", senderListenAddr)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer pc.Close()

	// Listen loop
	for {
		packet := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(packet)
		if err != nil {
		fmt.Println(err.Error())
		}

		// Handle receving packet
		fmt.Println(n, addr.String(), packet)
	}
}

// Initiate to establish connection
func initConn() {

}

func generateSeq() uint16 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	seqNum := r.Intn(0x01 << 15)

	return uint16(seqNum)
}

func main() {
	pkt := []byte("Hello Test")
	sendUDP(pkt)
}








