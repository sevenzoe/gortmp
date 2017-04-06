package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"../util"
)

// Action Message Format -- AMF 0
// Action Message Format -- AMF 3
// http://download.macromedia.com/pub/labs/amf/amf0_spec_121207.pdf
// http://wwwimages.adobe.com/www.adobe.com/content/dam/Adobe/en/devnet/amf/pdf/amf-file-format-spec.pdf

// AMF Object == AMF Object Type(1 byte) + AMF Object Value
//
// AMF Object Value :
// AMF0_STRING : 2 bytes(datasize,记录string的长度) + data(string)
// AMF0_OBJECT : AMF0_STRING + AMF Object
// AMF0_NULL : 0 byte
// AMF0_NUMBER : 8 bytes
// AMF0_DATE : 10 bytes
// AMF0_BOOLEAN : 1 byte
// AMF0_ECMA_ARRAY : 4 bytes(arraysize,记录数组的长度) + AMF0_OBJECT
// AMF0_STRICT_ARRAY : 4 bytes(arraysize,记录数组的长度) + AMF Object

// 实际测试时,AMF0_ECMA_ARRAY数据如下:
// 8 0 0 0 13 0 8 100 117 114 97 116 105 111 110 0 0 0 0 0 0 0 0 0 0 5 119 105 100 116 104 0 64 158 0 0 0 0 0 0 0 6 104 101 105 103 104 116 0 64 144 224 0 0 0 0 0
// 8 0 0 0 13 | { 0 8 100 117 114 97 116 105 111 110 --- 0 0 0 0 0 0 0 0 0 } | { 0 5 119 105 100 116 104 --- 0 64 158 0 0 0 0 0 0 } | { 0 6 104 101 105 103 104 116 --- 0 64 144 224 0 0 0 0 0 } |...
// 13 | {AMF0_STRING --- AMF0_NUMBER} | {AMF0_STRING --- AMF0_NUMBER} | {AMF0_STRING --- AMF0_NUMBER} | ...
// 13 | {AMF0_OBJECT} | {AMF0_OBJECT} | {AMF0_OBJECT} | ...
// 13 | {duration --- 0} | {width --- 1920} | {height --- 1080} | ...

const (
	AMF0_NUMBER         = 0x00 // 浮点数
	AMF0_BOOLEAN        = 0x01 // 布尔型
	AMF0_STRING         = 0x02 // 字符串
	AMF0_OBJECT         = 0x03 // 对象,开始
	AMF0_MOVIECLIP      = 0x04
	AMF0_NULL           = 0x05 // null
	AMF0_UNDEFINED      = 0x06
	AMF0_REFERENCE      = 0x07
	AMF0_ECMA_ARRAY     = 0x08
	AMF0_END_OBJECT     = 0x09 // 对象,结束
	AMF0_STRICT_ARRAY   = 0x0A
	AMF0_DATE           = 0x0B // 日期
	AMF0_LONG_STRING    = 0x0C // 字符串
	AMF0_UNSUPPORTED    = 0x0D
	AMF0_RECORDSET      = 0x0E
	AMF0_XML_DOCUMENT   = 0x0F
	AMF0_TYPED_OBJECT   = 0x10
	AMF0_AVMPLUS_OBJECT = 0x11

	AMF3_UNDEFINED     = 0x00
	AMF3_NULL          = 0x01
	AMF3_FALSE         = 0x02
	AMF3_TRUE          = 0x03
	AMF3_INTEGER       = 0x04
	AMF3_DOUBLE        = 0x05
	AMF3_STRING        = 0x06
	AMF3_XML_DOC       = 0x07
	AMF3_DATE          = 0x08
	AMF3_ARRAY         = 0x09
	AMF3_OBJECT        = 0x0A
	AMF3_XML           = 0x0B
	AMF3_BYTE_ARRAY    = 0x0C
	AMF3_VECTOR_INT    = 0x0D
	AMF3_VECTOR_UINT   = 0x0E
	AMF3_VECTOR_DOUBLE = 0x0F
	AMF3_VECTOR_OBJECT = 0x10
	AMF3_DICTIONARY    = 0x11
)

type AMFObject interface{}

type AMFObjects map[string]AMFObject

func newAMFObjects() AMFObjects {
	return make(AMFObjects, 0)
}

func decodeAMFObject(obj interface{}, key string) interface{} {
	if v, ok := obj.(AMFObjects)[key]; ok {
		return v
	}
	return nil
}

type AMF struct {
	out *bytes.Buffer
	in  *bytes.Buffer
}

func newAMFEncoder() (amf *AMF) {
	amf = new(AMF)
	amf.out = new(bytes.Buffer)
	return amf
}

func newAMFDecoder(b []byte) (amf *AMF) {
	amf = new(AMF)
	amf.in = bytes.NewBuffer(b)
	return amf
}

func (amf *AMF) readObjects() (obj []AMFObject, err error) {
	obj = make([]AMFObject, 0)

	for amf.in.Len() > 0 {
		v, err := amf.decodeObject()
		if err != nil {
			break
		}
		obj = append(obj, v)
	}

	return
}

func (amf *AMF) writeObjects(obj []AMFObject) error {
	for _, v := range obj {
		switch data := v.(type) {
		case string:
			{
				amf.writeString(data)
			}
		case float64:
			{
				amf.writeNumber(data)
			}
		case bool:
			{
				amf.writeBool(data)
			}
		case AMFObjects:
			{
				amf.encodeObject(data)
			}
		case nil:
			{
				amf.writeNull()
			}
		default:
			{
				fmt.Println("unknow type")
			}
		}
	}

	return nil
}

func (amf *AMF) decodeObject() (obj AMFObject, err error) {
	buf := amf.in

	if buf.Len() == 0 {
		return nil, errors.New(fmt.Sprintf("no enough bytes, %v/%v", buf.Len(), 1))
	}

	t, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	err = buf.UnreadByte()
	if err != nil {
		return nil, err
	}

	switch t {
	case AMF0_NUMBER:
		{
			return amf.readNumber()
		}
	case AMF0_BOOLEAN:
		{
			return amf.readBool()
		}
	case AMF0_STRING:
		{
			return amf.readString()
		}
	case AMF0_OBJECT:
		{
			return amf.readObject()
		}
	case AMF0_MOVIECLIP:
		{
			fmt.Println("This type is not supported and is reserved for future use.(AMF0_MOVIECLIP)")
		}
	case AMF0_NULL:
		{
			return amf.readNull()
		}
	case AMF0_UNDEFINED:
		{
			buf.ReadByte()
			return "Undefined", nil
		}
	case AMF0_REFERENCE:
		{
			fmt.Println("reference-type.(AMF0_REFERENCE)")
		}
	case AMF0_ECMA_ARRAY:
		{
			return amf.readECMAArray()
		}
	case AMF0_END_OBJECT:
		{
			buf.ReadByte()
			return "ObjectEnd", nil
		}
	case AMF0_STRICT_ARRAY:
		{
			return amf.readStrictArray()
		}
	case AMF0_DATE:
		{
			return amf.readDate()
		}
	case AMF0_LONG_STRING:
		{
			return amf.readLongString()
		}
	case AMF0_UNSUPPORTED:
		{
			fmt.Println("If a type cannot be serialized a special unsupported marker can be used in place of the type.(AMF0_UNSUPPORTED)")
		}
	case AMF0_RECORDSET:
		{
			fmt.Println("This type is not supported and is reserved for future use.(AMF0_RECORDSET)")
		}
	case AMF0_XML_DOCUMENT:
		{
			return amf.readLongString()
		}
	case AMF0_TYPED_OBJECT:
		{
			fmt.Println("If a strongly typed object has an alias registered for its class then the type name will also be serialized. Typed objects are considered complex types and reoccurring instances can be sent by reference.(AMF0_TYPED_OBJECT)")
		}
	case AMF0_AVMPLUS_OBJECT:
		{
			fmt.Println("AMF0_AVMPLUS_OBJECT")
		}
	default:
		{
			fmt.Println("Unkonw type.")
		}
	}

	return nil, errors.New(fmt.Sprintf("Unsupported type %v", t))
}

func (amf *AMF) encodeObject(t AMFObjects) (err error) {
	amf.writeObject()

	for k, vv := range t {
		if vvv, ok := vv.(string); ok {
			err = amf.writeObjectString(k, vvv)
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(float64); ok {
			err = amf.writeObjectNumber(k, vvv)
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(bool); ok {
			err = amf.writeObjectBool(k, vvv)
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(int); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(int16); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(int32); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(int64); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(uint16); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(uint32); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(uint64); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(uint8); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(int8); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		} else if vvv, ok := vv.(byte); ok {
			err = amf.writeObjectNumber(k, float64(vvv))
			if err != nil {
				break
			}
		}
	}

	amf.writeObjectEnd()

	return
}

func (amf *AMF) readDate() (t uint64, err error) {
	buf := amf.in

	buf.ReadByte()              // 取出第一个字节 8 Bit == 1 Byte. buf - 1.
	b, err := readBytes(buf, 8) // 在取出8个字节,并且读到b中. buf - 8
	t = util.BigEndian.Uint64(b)
	b, err = readBytes(buf, 2)

	return t, err
}

func (amf *AMF) readStrictArray() (list []AMFObject, err error) {
	buf := amf.in
	list = make([]AMFObject, 0)

	buf.ReadByte()
	b, err := readBytes(buf, 4)

	size := int(util.BigEndian.Uint32(b))
	for i := 0; i < size; i++ {
		obj, err := amf.decodeObject()
		if err != nil {
			break
		}

		list = append(list, obj)
	}

	return
}

func (amf *AMF) readECMAArray() (m AMFObjects, err error) {
	buf := amf.in
	m = make(AMFObjects, 0)

	buf.ReadByte()
	b, err := readBytes(buf, 4)
	size := int(util.BigEndian.Uint32(b))

	for i := 0; i < size; i++ {
		k, err := amf.readString1()
		if err != nil {
			break
		}

		v, err := amf.decodeObject()
		if err != nil {
			break
		}

		if k == "" && "ObjectEnd" == v {
			break
		}

		m[k] = v
	}

	return m, err
}

func (amf *AMF) readString() (str string, err error) {
	buf := amf.in

	buf.ReadByte()                  // 取出第一个字节 8 Bit == 1 Byte. buf - 1.
	b, err := readBytes(buf, 2)     // 在取出2个字节,并且读到b中. buf - 2
	l := util.BigEndian.Uint16(b)   // 大端
	b, err = readBytes(buf, int(l)) // 读取全部数据,读取长度为l,因为这两个字节(l变量)保存的是数据长度

	return string(b), err
}

func (amf *AMF) readString1() (str string, err error) {
	buf := amf.in

	b, err := readBytes(buf, 2)
	l := util.BigEndian.Uint16(b)
	b, err = readBytes(buf, int(l))

	return string(b), err
}

func (amf *AMF) readLongString() (str string, err error) {
	buf := amf.in

	buf.ReadByte()
	b, err := readBytes(buf, 4)
	l := util.BigEndian.Uint32(b)
	b, err = readBytes(buf, int(l))

	return string(b), err
}

func (amf *AMF) readNull() (AMFObject, error) {
	amf.in.ReadByte()
	return nil, nil
}

func (amf *AMF) readNumber() (num float64, err error) {
	buf := amf.in

	// binary.read 会读取8个字节(float64),如果小于8个字节返回一个`io.ErrUnexpectedEOF`,如果大于就会返回`io.ErrShortBuffer`,读取完毕会有`io.EOF`
	buf.ReadByte()
	err = binary.Read(buf, binary.BigEndian, &num)

	return num, err
}

func (amf *AMF) readBool() (f bool, err error) {
	buf := amf.in

	buf.ReadByte()
	b, err := buf.ReadByte()
	if b == 1 {
		f = true
	}

	return f, err
}

func (amf *AMF) readObject() (m AMFObjects, err error) {
	buf := amf.in

	buf.ReadByte()
	m = make(AMFObjects, 0)

	for {
		k, err := amf.readString1()
		if err != nil {
			break
		}

		v, err := amf.decodeObject()
		if err != nil {
			break
		}

		if k == "" && "ObjectEnd" == v {
			break
		}

		m[k] = v
	}

	return m, err
}

func readBytes(buf *bytes.Buffer, length int) (b []byte, err error) {
	b = make([]byte, length)

	i, err := buf.Read(b)
	if length != i {
		err = errors.New(fmt.Sprintf("not enough bytes,%v/%v", buf.Len(), length))
	}

	return
}

func (amf *AMF) writeString(value string) error {
	buf := amf.out
	v := []byte(value)

	err := buf.WriteByte(byte(AMF0_STRING))
	if err != nil {
		return err
	}

	b := make([]byte, 2)
	util.BigEndian.PutUint16(b, uint16(len(v)))
	_, err = buf.Write(b)
	if err != nil {
		return err
	}

	_, err = buf.Write(v)

	return err
}

func (amf *AMF) writeNull() error {
	buf := amf.out

	return buf.WriteByte(byte(AMF0_NULL))
}

func (amf *AMF) writeBool(b bool) error {
	buf := amf.out

	err := buf.WriteByte(byte(AMF0_BOOLEAN))
	if err != nil {
		return err
	}

	if b {
		return buf.WriteByte(byte(1))
	}

	return buf.WriteByte(byte(0))
}

func (amf *AMF) writeNumber(b float64) error {
	buf := amf.out

	err := buf.WriteByte(byte(AMF0_NUMBER))
	if err != nil {
		return err
	}

	return binary.Write(buf, binary.BigEndian, b)
}

func (amf *AMF) writeObject() error {
	buf := amf.out

	return buf.WriteByte(byte(AMF0_OBJECT))
}

func (amf *AMF) writeObjectString(key, value string) error {
	buf := amf.out
	b := make([]byte, 2)

	util.BigEndian.PutUint16(b, uint16(len([]byte(key))))

	_, err := buf.Write(b)
	if err != nil {
		return err
	}

	_, err = buf.Write([]byte(key))
	if err != nil {
		return err
	}

	return amf.writeString(value)
}

func (amf *AMF) writeObjectBool(key string, f bool) error {
	buf := amf.out
	b := make([]byte, 2)

	util.BigEndian.PutUint16(b, uint16(len([]byte(key))))

	_, err := buf.Write(b)
	if err != nil {
		return err
	}

	_, err = buf.Write([]byte(key))
	if err != nil {
		return err
	}

	return amf.writeBool(f)
}

func (amf *AMF) writeObjectNumber(key string, value float64) error {
	buf := amf.out
	b := make([]byte, 2)

	util.BigEndian.PutUint16(b, uint16(len([]byte(key))))

	_, err := buf.Write(b)
	if err != nil {
		return err
	}

	_, err = buf.Write([]byte(key))
	if err != nil {
		return err
	}

	return amf.writeNumber(value)
}

func (amf *AMF) writeObjectEnd() error {
	buf := amf.out

	err := buf.WriteByte(byte(0))
	if err != nil {
		return err
	}

	err = buf.WriteByte(byte(0))
	if err != nil {
		return err
	}

	return buf.WriteByte(byte(AMF0_END_OBJECT))
}

func (amf *AMF) Bytes() []byte {
	return amf.out.Bytes()
}
