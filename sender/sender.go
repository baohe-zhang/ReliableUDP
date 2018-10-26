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

var lastAckSeq uint32
var syncSeqNum uint32
var curSeqNum uint32
var cwndSize uint16
var cwndBase uint32
var dupAckCount int

var expectedSeqNum uint32
var rwndSize uint16
var rwndBase uint32

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1 << 10)

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

	if header.Flags&ACK != 0 {
		fmt.Println("Received ACK with AckNum: ", header.AckNum)

		if header.AckNum > lastAckSeq {
			offset := header.AckNum - lastAckSeq
			lastAckSeq = header.AckNum
			// Data is Acknowledged, increase cwnd and slide cwnd
			if cwndSize < header.WinSize {
				cwndSize += uint16(offset)
				if cwndSize > header.WinSize {
					cwndSize = header.WinSize
				}
				fmt.Println("Congestion window size update to ", cwndSize/MSS)
			}
			cwndBase += offset
			fmt.Println("Congestion window base update to ", cwndBase/MSS)
		} else if header.AckNum == lastAckSeq {
			// Duplicated ACK
			dupAckCount += 1
			if dupAckCount == 3 {
				fmt.Println("Congestion occurs. Receive 3 DupACKs.")
			}
		}
	}

	// Receive data
	if len(packet) > HeaderLength {
		// Check whether this packet is expected
		if header.SeqNum == expectedSeqNum {
			payload := packet[HeaderLength:]
			sendACK(header.SeqNum, len(payload))
		} else if header.SeqNum > expectedSeqNum {
			fmt.Println("Receive out of order packet with seq: ", header.SeqNum)
			// Buffer the packet and send duplicate ACK
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
		WinSize: 40960,
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
		WinSize: 40960,
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
		WinSize: 40960,
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

	syncSeqNum = 32329

	curSeqNum = syncSeqNum
	lastAckSeq = syncSeqNum
	cwndSize = 4 * MSS
	cwndBase = 0


	data := make([]byte, 100*MSS)

	for {
		offset := curSeqNum - syncSeqNum
		if offset-cwndBase < uint32(cwndSize) && offset < uint32(len(data)) {
			sendData(data[offset : offset+MSS])
		}
	}
}
