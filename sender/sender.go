package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"
)

const (
	SYN        = 0X01 << 1
	ACK        = 0X01 << 2
	FIN        = 0X01 << 3
	SourceIP   = "127.0.0.1"
	SourcePort = 20002
	TargetIP   = "127.0.0.1"
	TargetPort = 8080
	HeaderLength = 18
)

type Header struct {
	srcIP   uint32
	srcPort uint16
	seqNum  uint32
	ackNum  uint32
	flags   uint16
	winSize uint16
}

var curSeqNum uint32

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	seqNum := r.Intn(1 << 16)

	return uint32(seqNum)
}

func sendUDP(packet []byte) {
	conn, err := net.Dial("udp", TargetIP+":"+strconv.Itoa(TargetPort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer conn.Close()

	conn.Write(packet)
}

func recvUDP() {
	pc, err := net.ListenPacket("udp", TargetIP+":"+strconv.Itoa(TargetPort))
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
		fmt.Printf("%s %v", addr.String(), packet[:n])
		packetHandler(packet[:n])
	}
}

func packetHandler(packet []byte) {
	// Deserialize header
	var header Header
	headerBytes := packet[:HeaderLength]
	headerBuf := bytes.NewReader(headerBytes)
	err = binary.Read(headerBuf, binary.BigEndian, &header)
	if err != nil {
			fmt.Println(err.Error())
	}

	if header.flags & SYN != 0 {

	}

	if header.flags & ACK != 0 {

	}

	if header.flags & FIN != 0 {
		
	}

	if (len(packet) > HeaderLength) {
		payload := packet[HeaderLength:]
		fmt.Printf("%v", payload)
	}
}


// Initiate a connection
func initConn() {
	curSeqNum = generateSeqNum()

	header := Header{
		srcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		srcPort: SourcePort,
		seqNum:  curSeqNum,
		ackNum:  0x00,
		flags:   SYN,
		winSize: uint16(4096),
	}

	// Serialize the header
	headerBuf := bytes.Buffer{}
	binary.Write(&headerBuf, binary.BigEndian, header)

	sendUDP(headerBuf.Bytes())
}

func closeConn() {

}

func sendData() {
	header := Header{
		srcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		srcPort: SourcePort,
		seqNum:  curSeqNum,
		ackNum:  0x00,
		flags:   0x00,
		winSize: uint16(4096),
	}

	// Serialize the header
	headerBuf := bytes.Buffer{}
	binary.Write(&headerBuf, binary.BigEndian, header)

	sendUDP(headerBuf.Bytes())
}

func sendACK(_header Header) {

}

func main() {
	initConn()
}
