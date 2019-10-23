package socket

import (
	"bufio"
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
)

// 协议中用到的常量
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

// Enpack 封包
func Enpack(pushType, messageId, source, topic string, content []byte) ([]byte, error) {
	if pushType == "" {
		return nil, errors.New("type is empty")
	}
	if topic == "" {
		return nil, errors.New("topic is empty")
	}
	if content == nil {
		return nil, errors.New("content is empty")
	}
	// ConstHeaderConstIntLengthData
	// data = pushType+ConstSplitSpace+topic+ConstSplitSpace+content
	res := []byte(pushType)
	res = append(res, []byte(ConstSplitSpace)...)
	res = append(res, []byte(messageId)...)
	res = append(res, []byte(ConstSplitSpace)...)
	res = append(res, []byte(source)...)
	res = append(res, []byte(ConstSplitSpace)...)
	res = append(res, []byte(topic)...)
	res = append(res, ConstNewLine)
	res = append(res, content...)
	return append(append([]byte(ConstHeader), IntToBytes(len(res))...), res...), nil
}

// Depack 解包
func Depack(buffer []byte, readerChannel chan *SendMessage) ([]byte, error) {
	length := len(buffer)
	var (
		i   int
		err error
	)
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
			// params[1] = messageId
			// params[2] = source
			// params[3] = topic
			// params[4] = content
			var (
				params  [][]byte
				param   []byte
				content = make([]byte, messageLength)
				n       int
			)

			param, _, err = reader.ReadLine()
			if err != nil {
				break
			}

			params = bytes.Split(param, []byte(ConstSplitSpace))
			n, err = reader.Read(content)
			if err != nil {
				break
			}
			params = append(params, content[:n])

			if len(params) < 3 {
				// 包解析发生错误
				err = errors.New("包解析出错")
				goto Error
			}
			sendMessage := GetSendMessage(string(params[1]), string(params[2]))
			sendMessage.Type = string(params[0])
			sendMessage.Topic = string(params[3])
			sendMessage.Data = params[4]
			readerChannel <- sendMessage
		}
	}
	if i == length {
		return make([]byte, 0), nil
	}
	return buffer[i:], nil // 返回的是粘包多余的部分
Error:
	return buffer[i:], err
}

// IntToBytes 整形转换成字节
func IntToBytes(n int) []byte {
	x := int32(n)

	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

// BytesToInt 字节转换成整形
func BytesToInt(b []byte) int {
	bytesBuffer := bytes.NewBuffer(b)

	var x int32
	binary.Read(bytesBuffer, binary.BigEndian, &x)

	return int(x)
}
