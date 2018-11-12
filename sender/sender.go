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
	SlowStart = 0
	CongestionAvoidance = 1
	FastRecovery = 2
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	DATA         = 0X01 << 4
	HeaderLength = 12
	MSS          = 1024
	RTO          = 100 * time.Millisecond
	DestAddr     = "127.0.0.1:8080"
)

type Header struct {
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

// Sender's variables
var state uint8
var syncSeqNum uint32
var curAckNum uint32
var nextSeqNum uint32
var cwndSize uint16
var cwndBase uint32
var ssthresh uint16
var dupAckCount uint8
var isCongestion bool
var culmulativeAckBytes uint16
var timer *time.Timer
var ch chan uint32
var winUpdate chan bool
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

		if header.AckNum == syncSeqNum {
			// Connection request accepted, notify sending goroutine
			ch <- syncSeqNum
		}

		if header.AckNum > curAckNum {
			// New ACK
			fmt.Println("receive new ACK: ", header.AckNum/MSS)
			AckBytes := header.AckNum - curAckNum
			if state == FastRecovery {
				dupAckCount = 0
				state = CongestionAvoidance
				cwndSize = ssthresh
				fmt.Printf("[fast reocovery] cw set to %d\n", cwndSize/MSS)

			} else if state == SlowStart {
				dupAckCount = 0
				if cwndSize <= header.WinSize - uint16(AckBytes) {
					cwndSize += uint16(AckBytes)
					fmt.Printf("[slow state] cw increase to %d\n", cwndSize/MSS)
				}
				if cwndSize >= ssthresh {
					state = CongestionAvoidance
				}

			} else if state == CongestionAvoidance {
				dupAckCount = 0
				culmulativeAckBytes += uint16(AckBytes)
				if culmulativeAckBytes >= cwndSize && cwndSize <= header.WinSize - MSS {
					culmulativeAckBytes = 0
					cwndSize += MSS
					fmt.Printf("[congestion avoidance] cw increase to %d\n", cwndSize/MSS)
				}
			} else {
				fmt.Println("error state")
			}
			curAckNum = header.AckNum
			cwndBase += AckBytes
			winUpdate <- true
			fmt.Printf("[slide window] cw base set to %d\n", cwndBase/MSS)
			// Restatr timer
			stop := timer.Stop()
			if stop {
				startTimer()
			}


		} else if header.AckNum == curAckNum {
			// Duplicate ACK
			fmt.Println("receive duplicate ACK: ", header.AckNum/MSS)
			if state == FastRecovery && cwndSize <= header.WinSize - MSS {
				cwndSize += MSS
				fmt.Printf("[fast reocovery] cw set to %d\n", cwndSize/MSS)

			} else if state == SlowStart {
				dupAckCount++
				if dupAckCount == 3 {
					state = FastRecovery
					ssthresh = cwndSize / 2
					if ssthresh < 2 * MSS {
						ssthresh = 2 * MSS
					}
					cwndSize = ssthresh + 3 * MSS
					ch <- curAckNum // Retransmit missing packet
					fmt.Printf("receive 3 dup Acks, ssthresh set to %d, cw size set to %d, retransmit %d\n", ssthresh, cwndSize, curAckNum)
				}
			} else if state == CongestionAvoidance {
				dupAckCount++
				if dupAckCount == 3{
					state = FastRecovery
					ssthresh = cwndSize / 2
					if ssthresh < 2 * MSS {
						ssthresh = 2 * MSS
					}
					cwndSize = ssthresh + 3 * MSS
					ch <- curAckNum
					fmt.Printf("receive 3 dup Acks, ssthresh set to %d, cw size set to %d, retransmit %d\n", ssthresh, cwndSize, curAckNum)
				}
			}
			winUpdate <- true

		} else {
			// Receive Ack number less than current Ack number, do nothing
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
	state = SlowStart
	syncSeqNum = 0
	nextSeqNum = syncSeqNum
	curAckNum = syncSeqNum
	cwndSize = 1 * MSS
	cwndBase = 0
	ssthresh = 32 * MSS
	dupAckCount = 0
	ch = make(chan uint32)
	winUpdate = make(chan bool)

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

	start := time.Now()

	for {
		select {

		case seqNum := <-ch:
			// Retransmit seqNum
			bytePos := seqNum - syncSeqNum
			if bytePos+MSS < filesize {
				sendDATA(conn, seqNum, fileBytes[bytePos:bytePos+MSS])
			} else {
				sendDATA(conn, seqNum, fileBytes[bytePos:filesize])
			}

		case <- winUpdate:
			bytePos := nextSeqNum - syncSeqNum
			if bytePos-cwndBase < uint32(cwndSize) {
				if bytePos+MSS < filesize {
					sendDATA(conn, nextSeqNum, fileBytes[bytePos:bytePos+MSS])
					nextSeqNum = syncSeqNum + bytePos + MSS
				} else {
					sendDATA(conn, nextSeqNum, fileBytes[bytePos:filesize])
					nextSeqNum = syncSeqNum + bytePos + (filesize - bytePos)
				}
			}

		default:

		}

		if cwndBase == filesize {
			fmt.Printf("%s transfer finish\n", filename)
			break
		}
	}

	end := time.Now()
	t := end.Sub(start)
	fmt.Printf("average bandwidth %f MB/s\n", float64(filesize) / t.Seconds() / 100000)
}

func main() {
	sendFile("bfile")
}