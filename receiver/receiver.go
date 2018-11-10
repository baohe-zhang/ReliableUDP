package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	DATA         = 0x01 << 4
	HeaderLength = 12
	MSS          = 1024
	SrcAddr      = "127.0.0.1:8080"
)

type Header struct {
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

// Receiver's variables
var remoteAddr net.Addr
var expectedSeqNum uint32
var rwndSize uint16
var rwndBase uint32
var bufSeqNumSet map[uint32]bool
var bufPayloadMap map[uint32][]byte
var ch chan uint32
var addrch chan net.Addr
var pcch chan net.PacketConn

func BinaryIP(stringIP string) uint32 {
	return binary.BigEndian.Uint32(net.ParseIP(stringIP).To4())
}

func StringIP(binaryIP uint32) string {
	IP := make(net.IP, 4)
	binary.BigEndian.PutUint32(IP, binaryIP)
	return IP.String()
}

func packetListener(localAddr string) {
	pc, err := net.ListenPacket("udp", localAddr)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer pc.Close()

	pcch <- pc // Pass the pc to the receiver goroutine

	// Listen loop
	packet := make([]byte, 1536)
	for {
		n, addr, err := pc.ReadFrom(packet)
		fmt.Println("receive")
		if err != nil {
			fmt.Println(err.Error())
		}

		// Handle receving packet
		packetHandler(packet[:n], addr)
	}
}

func packetHandler(packet []byte, addr net.Addr) {
	// Deserialize header
	var header Header
	headerBytes := packet[:HeaderLength]
	headerBuf := bytes.NewReader(headerBytes)
	err := binary.Read(headerBuf, binary.BigEndian, &header)
	if err != nil {
		fmt.Println(err.Error())
	}

	if header.Flags&SYN != 0 {
		fmt.Printf("Receive SYN from %s with SeqNum: %d\n", addr.String(), header.SeqNum)

		ch <- header.SeqNum
		addrch <- addr
	}

	if header.Flags&FIN != 0 {

	}

	if header.Flags&DATA != 0 {

		if header.SeqNum == expectedSeqNum {
			fmt.Println("[In Order] receive packet with SeqNum: ", expectedSeqNum/MSS)

			// Deliver payload
			payload := packet[HeaderLength:]
			payloadSize := uint32(len(payload))
			fmt.Printf("deliver %d bytes of seqNum %d\n", payloadSize, expectedSeqNum/MSS)

			culmulativePayloadSize := payloadSize
			AckNum := expectedSeqNum + culmulativePayloadSize

			// Check if there are consecutive buffered ACK
			_, ok := bufSeqNumSet[AckNum]
			if ok {
				delete(bufSeqNumSet, AckNum)
			}

			for ok {
				// Deliver buffered payload
				bufPayload := bufPayloadMap[AckNum]
				bufPayloadSize := uint32(len(bufPayload))
				fmt.Printf("deliver %d bytes of seqNum %d\n", bufPayloadSize, AckNum/MSS)
				delete(bufPayloadMap, AckNum)

				culmulativePayloadSize += bufPayloadSize
				AckNum = expectedSeqNum + culmulativePayloadSize

				// Check if there are consecutive buffered ACK
				_, ok = bufSeqNumSet[AckNum]
				if ok {
					delete(bufSeqNumSet, AckNum)
				}
			}

			// Send ACK and update expectedSeqNum
			ch <- AckNum
			expectedSeqNum = AckNum

		} else if header.SeqNum > expectedSeqNum {
			fmt.Println("[Out of Order] receive packet with SeqNum: ", header.SeqNum/MSS)
			// Buffer payload
			payload := packet[HeaderLength:]
			bufPayloadMap[header.SeqNum] = payload

			// Mark the out of order packet
			bufSeqNumSet[header.SeqNum] = true

			// Send duplicate ACK
			ch <- expectedSeqNum

		} else {
			fmt.Println("[Already Acknowledged] Receive Acked packet with SeqNum: ", header.SeqNum/MSS)
		}
	}
}

func sendACK(pc *net.PacketConn, remoteAddr net.Addr, AckNum uint32) {
	header := Header{
		SeqNum:  0x00,
		AckNum:  AckNum,
		Flags:   ACK,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	(*pc).WriteTo(buf.Bytes(), remoteAddr)
	fmt.Println("Send ACK with AckNum: ", AckNum/MSS)
}

func recvFile() {
	bufSeqNumSet = map[uint32]bool{}    // Mark out of order packet
	bufPayloadMap = map[uint32][]byte{} // Buffer out of order packet's payload
	ch = make(chan uint32)
	addrch = make(chan net.Addr)
	pcch = make(chan net.PacketConn)

	// Setup listening packet connection
	go packetListener(SrcAddr)
	pc := <- pcch
	fmt.Println("start listening at ", pc.LocalAddr().String())

	// Receive SYN
	syncSeqNum := <-ch
	remoteAddr = <- addrch
	expectedSeqNum = syncSeqNum

	// Send ACK to accept connection
	sendACK(&pc, remoteAddr, syncSeqNum)

	isFinish := false

	for {
		select {
		case AckNum := <-ch:
			sendACK(&pc, remoteAddr, AckNum)
			if AckNum%MSS != 0 {
				isFinish = true
			}

		default:
		}

		if isFinish {
			fmt.Println("receive finish")
			time.Sleep(100 * time.Millisecond)
			break
		}
	}
}

func main() {
	recvFile()
}
