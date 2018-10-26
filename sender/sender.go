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
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	SourceIP     = "127.0.0.1"
	SourcePort   = 20002
	TargetIP     = "127.0.0.1"
	TargetPort   = 8080
	HeaderLength = 18
	MSS          = 1024
)

type Header struct {
	SrcIP   uint32
	SrcPort uint16
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

var curSeqNum uint32
var cwndSize uint32
var cwndBase uint32

var expectedSeqNum uint32
var rwndSize uint32
var rwndBase uint32

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1 << 16)

	return uint32(SeqNum)
}

func sendUDP(packet []byte) {
	conn, err := net.Dial("udp", TargetIP+":"+strconv.Itoa(TargetPort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer conn.Close()

	conn.Write(packet)
}

func packetListener() {
	pc, err := net.ListenPacket("udp", SourceIP+":"+strconv.Itoa(SourcePort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer pc.Close()

	// Listen loop
	for {
		packet := make([]byte, 1536)
		n, _, err := pc.ReadFrom(packet)
		if err != nil {
			fmt.Println(err.Error())
		}

		// Handle receving packet
		packetHandler(packet[:n])
	}
}

func packetHandler(packet []byte) {
	// Deserialize header
	var header Header
	headerBytes := packet[:HeaderLength]
	headerBuf := bytes.NewReader(headerBytes)
	err := binary.Read(headerBuf, binary.BigEndian, &header)
	if err != nil {
		fmt.Println(err.Error())
	}

	// SrcIP := header.SrcIP
	// SrcPort := header.SrcPort
	// SeqNum := header.SeqNum
	// fmt.Println("Received Packet with ", SrcIP, SrcPort, SeqNum)

	if header.Flags&SYN != 0 {

	}

	if header.Flags&FIN != 0 {

	}

	// Data is Acknowledged, slide cwnd
	if header.Flags&ACK != 0 {
		cwndBase = header.AckNum
		fmt.Println("Update cwndBase to ", cwndBase)
	}

	// Receive data
	if len(packet) > HeaderLength {
		// Check whether this packet is expected
		if header.SeqNum == expectedSeqNum {
			payload := packet[HeaderLength:]
			sendACK(header.SeqNum, len(payload))
		} else if header.SeqNum > expectedSeqNum {
			fmt.Println("Receive out of order packet with seq: ", header.SeqNum)
			// Buffer the packet, send duplicate ACK
			sendACK(expectedSeqNum, 0)
		}
	}
}

// Initiate a connection
func initConn() {
	curSeqNum = generateSeqNum()

	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  curSeqNum,
		AckNum:  0x00,
		Flags:   SYN,
		WinSize: uint16(4096),
	}

	// Serialize the header
	headerBuf := bytes.Buffer{}
	binary.Write(&headerBuf, binary.BigEndian, header)

	sendUDP(headerBuf.Bytes())
}

func closeConn() {

}

func sendData(data []byte) {
	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  curSeqNum,
		AckNum:  0x00,
		Flags:   0x00,
		WinSize: uint16(4096),
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	buf.Write(data)

	sendUDP(buf.Bytes())
	fmt.Println("Send data with SeqNum: ", curSeqNum)

	// Update current sequence number
	curSeqNum = ((curSeqNum + uint32(len(data))) % (0x01 << 31))
}

func sendACK(seqNum uint32, payloadLength int) {
	AckNum := uint32((seqNum + uint32(payloadLength)) % (0x01 << 31))

	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  0x00,
		AckNum:  AckNum,
		Flags:   ACK,
		WinSize: uint16(4096),
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	sendUDP(buf.Bytes())
	fmt.Println("Send ACK with AckNum: ", AckNum)

	// Update next expected sequence number
	expectedSeqNum = AckNum
}

func main() {
	go packetListener()

	curSeqNum = 0
	cwndSize = 4 * MSS
	cwndBase = 0

	data := make([]byte, 1000*MSS)

	for {
		if curSeqNum-cwndBase < cwndSize && curSeqNum+MSS <= uint32(len(data)) {
			sendData(data[curSeqNum : curSeqNum+MSS])
		}
	}
}
