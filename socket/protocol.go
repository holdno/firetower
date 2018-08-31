package socket

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	ConstHeader       = "FireHeader"
	ConstHeaderLength = 10
	ConstIntLength    = 4 // int转byte长度为4
	ConstSplitSpace   = " "
	ConstNewLine      = '\n'
)

// firetower protocol
// header+messageLength+[pushType]+ConstSplitSpace+[topic]+ConstNewLine+[content]
// |      header       |           type           |       params       |  body  |
// 封包
func Enpack(pushType, topic string, content []byte) []byte {
	// ConstHeaderConstIntLengthData
	// data = pushType+ConstSplitSpace+topic+ConstSplitSpace+content
	res := []byte(pushType)
	res = append(res, []byte(ConstSplitSpace)...)
	res = append(res, []byte(topic)...)
	res = append(res, ConstNewLine)
	res = append(res, content...)
	res = append(res, ConstNewLine)
	return append(append([]byte(ConstHeader), IntToBytes(len(res))...), res...)
}

//解包
func Depack(buffer []byte, readerChannel chan *PushMessage) []byte {
	length := len(buffer)
	var i int
	for i = 0; i < length; i = i + 1 {
		// 首先判断是否是一个完整的包
		if length < i+ConstHeaderLength+ConstIntLength {
			break
		}
		// 寻找包的开头
		if string(buffer[i:i+ConstHeaderLength]) == ConstHeader {
			messageLength := BytesToInt(buffer[i+ConstHeaderLength : i+ConstHeaderLength+ConstIntLength])
			if length < i+ConstHeaderLength+ConstIntLength+messageLength {
				break
			}
			data := buffer[i+ConstHeaderLength+ConstIntLength : i+ConstHeaderLength+ConstIntLength+messageLength]
			reader := bufio.NewReader(bytes.NewReader(data))
			// params[0] = pushType
			// params[1] = topic
			// params[2] = content
			var (
				params [][]byte
				n      = 0
			)
			for {
				if n == 3 {
					break // 包解析正常的话 第三次读取会触发 reader.ReadLine 的 err: io.EOF
				}
				param, _, err := reader.ReadLine()
				if err != nil {
					break
				}
				if n == 0 {
					params = bytes.Split(param, []byte(ConstSplitSpace))
				} else {
					params = append(params, param)
				}
				n++
			}

			if len(params) < 3 {
				// 包解析发生错误
				fmt.Printf("\n\n\n 严重错误\n包解析出错\n\n\n")
				break
			}

			readerChannel <- &PushMessage{
				Type:  string(params[0]),
				Topic: string(params[1]),
				Data:  params[2],
			}
		}
	}
	if i == length {
		return make([]byte, 0)
	}
	return buffer[i:] // 返回的是粘包多余的部分
}

//整形转换成字节
func IntToBytes(n int) []byte {
	x := int32(n)

	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

//字节转换成整形
func BytesToInt(b []byte) int {
	bytesBuffer := bytes.NewBuffer(b)

	var x int32
	binary.Read(bytesBuffer, binary.BigEndian, &x)

	return int(x)
}
