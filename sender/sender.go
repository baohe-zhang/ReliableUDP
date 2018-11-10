package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"time"
	// "sync"
)

const (
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	DATA         = 0X01 << 4
	HeaderLength = 12
	MSS          = 1024
	RTO          = 30 * time.Millisecond
	DestAddr     = "127.0.0.1:8080"
)

type Header struct {
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

// Sender's variables
var syncSeqNum uint32
var lastAckNum uint32
var nextSeqNum uint32
var cwndSize uint16
var cwndBase uint32
var ssthresh uint16
var dupAckCount uint8
var isCongestion bool
var culmulativeOffset uint16
var timer *time.Timer
var ch chan uint32
// var mutex sync.Mutex


func BinaryIP(stringIP string) uint32 {
	return binary.BigEndian.Uint32(net.ParseIP(stringIP).To4())
}

func StringIP(binaryIP uint32) string {
	IP := make(net.IP, 4)
	binary.BigEndian.PutUint32(IP, binaryIP)
	return IP.String()
}

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1000)

	return uint32(SeqNum)
}

func setupConn(addr string) *net.Conn {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		fmt.Println(err.Error())
	}

	return &conn
}

func packetListener(conn *net.Conn) {
	// Listen loop
	packet := make([]byte, 1536)
	for {

		// mutex.Lock()
		n, err := (*conn).Read(packet)
		if err != nil {
			fmt.Println(err.Error())
		}
		// mutex.Unlock()

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

	if header.Flags&FIN != 0 {

	}

	if header.Flags&ACK != 0 {
		fmt.Println("received ACK: ", header.AckNum/MSS)

		if header.AckNum == syncSeqNum {
			// Connection request accepted, notify sending goroutine
			ch <- syncSeqNum
		}

		if header.AckNum > lastAckNum {
			// New ACK
			dupAckCount = 0 // Reset duplicate ACK count
			offset := header.AckNum - lastAckNum
			lastAckNum = header.AckNum

			// Change congestion window size
			if isCongestion {
				// State transtion from Fast Recovery to Congestion Avodiance
				isCongestion = false
				cwndSize = ssthresh
				fmt.Println("[slow start] cw size set to ssthresh", ssthresh/MSS)

			} else if cwndSize <= ssthresh-uint16(offset) && cwndSize <= header.WinSize-uint16(offset) {
				// Slow Start
				cwndSize += uint16(offset)
				fmt.Println("[slow start] cw size increase to ", cwndSize/MSS)

			} else if cwndSize > ssthresh-uint16(offset) && cwndSize <= header.WinSize-uint16(offset) {
				// Congestion Avoidance
				culmulativeOffset += uint16(offset)
				if culmulativeOffset >= cwndSize {
					culmulativeOffset = 0
					cwndSize += MSS
					fmt.Println("[congestion avoidance] cw size increase to ", cwndSize/MSS)
				}
			}
			// Slide window
			cwndBase += offset
			fmt.Println("[slide window] cw base increase to ", cwndBase/MSS)

			// Restart RTO timer
			stop := timer.Stop()
			if stop {
				startTimer()
			}

		} else if header.AckNum == lastAckNum {
			// Duplicate ACK
			dupAckCount += 1
			if dupAckCount == 3 {
				isCongestion = true
				// Enter Fast Recovery state
				fmt.Println("[Congestion] receive 3 DupACKs.")
				ssthresh = cwndSize / 2
				fmt.Println("[Congestion] ssthresh set to ", ssthresh/MSS)
				cwndSize = ssthresh + 3*MSS
				fmt.Println("[Congestion] cw size decrease to ", cwndSize/MSS)
				// Notify sender goroutine to resend data through channel
				ch <- lastAckNum

			} else if dupAckCount > 3 {
				if cwndSize <= header.WinSize-MSS {
					cwndSize += MSS
					fmt.Println("[Fast Recovery] cw size increase to ", cwndSize/MSS)
				}
			}
		}
	}
}

func sendSYN(conn *net.Conn, seqNum uint32) {
	header := Header{
		SeqNum:  seqNum,
		AckNum:  0x00,
		Flags:   SYN,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	// mutex.Lock()
	(*conn).Write(buf.Bytes())
	// mutex.Unlock()

	fmt.Println("send SYN: ", seqNum/MSS)
}

func sendDATA(conn *net.Conn, seqNum uint32, data []byte) {
	header := Header{
		SeqNum:  seqNum,
		AckNum:  0x00,
		Flags:   DATA,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	// mutex.Lock()
	buf.Write(data)
	// mutex.Unlock()

	(*conn).Write(buf.Bytes())
	fmt.Println("send DATA: ", seqNum/MSS)
}

func startTimer() {
	timer = time.NewTimer(RTO)
	go func() {
		<-timer.C
		fmt.Println("[RTO timeout]")
		// Notify sender to retransmit
		ch <- (cwndBase - syncSeqNum)
		// Update variables
		ssthresh = cwndSize / 2
		cwndSize = 1 * MSS
		fmt.Println("[RTO timeout] cw size decrease to ", cwndSize/MSS)
		dupAckCount = 0
		// Start a new timer
		startTimer()
	}()
}

func sendFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer file.Close()

	var fileBuf bytes.Buffer
	io.Copy(&fileBuf, file)

	fileBytes := fileBuf.Bytes()
	filesize := uint32(len(fileBytes))

	fmt.Printf("%s's size: %d\n", filename, filesize)

	// Init sender's global variables
	syncSeqNum = 0
	nextSeqNum = syncSeqNum
	lastAckNum = syncSeqNum
	cwndSize = 1 * MSS
	cwndBase = 0
	ssthresh = 32 * MSS
	dupAckCount = 0
	ch = make(chan uint32)

	// Setup sending conn
	conn := setupConn(DestAddr)
	defer (*conn).Close()

	srcAddr := (*conn).LocalAddr().String()
	fmt.Println("source addr: ", srcAddr)

	// Start read goroutine
	go packetListener(conn)

	// Initiates a connection and block until receives the other side's ACK
	sendSYN(conn, syncSeqNum)
	code := <-ch
	for code != syncSeqNum {
	}

	// Init RTO timer
	startTimer()

	for {
		select {

		case seqNum := <-ch:
			// Retransmit seqNum
			offset := seqNum - syncSeqNum
			if offset+MSS < filesize {
				sendDATA(conn, seqNum, fileBytes[offset:offset+MSS])
			} else {
				sendDATA(conn, seqNum, fileBytes[offset:filesize])
			}

		default:
			offset := nextSeqNum - syncSeqNum
			if offset-cwndBase < uint32(cwndSize) {
				if offset+MSS < filesize {
					sendDATA(conn, nextSeqNum, fileBytes[offset:offset+MSS])
					nextSeqNum = syncSeqNum + offset + MSS
				} else {
					sendDATA(conn, nextSeqNum, fileBytes[offset:filesize])
					nextSeqNum = syncSeqNum + offset + (filesize - offset)
				}
			}
		}

		if cwndBase == filesize {
			fmt.Println("finish")
			break
		}
	}
}

func main() {
	sendFile("file")
}