package sc

import (
	"bytes"
	"fmt"
	"github.com/joeqian10/neo3-gogogo/helper"
	"github.com/joeqian10/neo3-gogogo/io"
	"go/types"
	"math/big"
	"strings"
)

type ScriptBuilder struct {
	buff *bytes.Buffer
	errs []error // new design, put all errors in the array
}

func (sb *ScriptBuilder) addError(err error) {
	if err != nil {
		sb.errs = append(sb.errs, err)
	}
}

func NewScriptBuilder() ScriptBuilder {
	return ScriptBuilder{
		buff: new(bytes.Buffer),
		errs: make([]error, 0),
	}
}

// ToArray converts ScriptBuilder to byte array, pop out all errors
func (sb *ScriptBuilder) ToArray() ([]byte, error) {
	if len(sb.errs) == 0 {
		return sb.buff.Bytes(), nil
	}

	var ss []string
	for _, err := range sb.errs {
		ss = append(ss, err.Error())
	}
	return sb.buff.Bytes(), fmt.Errorf(strings.Join(ss, "\n"))
}

func (sb *ScriptBuilder) Emit(op OpCode, arg ...byte) {
	err := sb.buff.WriteByte(byte(op))
	sb.addError(err)

	if arg != nil {
		_, err = sb.buff.Write(arg)
		sb.addError(err)
	}
}

func (sb *ScriptBuilder) EmitCall(offset int) {
	if offset < -128 || offset > 127 {
		sb.Emit(CALL_L, helper.IntToBytes(offset)...)
	} else {
		sb.Emit(CALL, byte(offset))
	}
}

func (sb *ScriptBuilder) EmitJump(op OpCode, offset int) {
	if op < JMP || op > JMPLE_L {
		sb.addError(fmt.Errorf("argument out of range: invalid OpCode"))
	}
	if int(op)%2 == 0 && (offset < -128 || offset > 127) {
		op += 1
	}
	if int(op)%2 == 0 {
		sb.Emit(op, byte(offset))
	} else {
		sb.Emit(op, helper.IntToBytes(offset)...)
	}
}

func (sb *ScriptBuilder) EmitPushBigInt(number *big.Int) {
	if number.Cmp(big.NewInt(-1)) >= 0 && number.Cmp(big.NewInt(16)) <= 0 { // >=-1 || <=16
		var b = byte(number.Int64())
		sb.Emit(PUSH0 + OpCode(b))
		return
	}
	// need little endian
	data := helper.BigIntToNeoBytes(number) // ToByteArray() returns big-endian
	if len(data) == 1 {
		sb.Emit(PUSHINT8, data...)
	} else if len(data) == 2 {
		sb.Emit(PUSHINT16, data...)
	} else if len(data) <= 4 {
		sb.Emit(PUSHINT32, helper.PadRight(data, 4)...)
	} else if len(data) <= 8 {
		sb.Emit(PUSHINT64, helper.PadRight(data, 8)...)
	} else if len(data) <= 16 {
		sb.Emit(PUSHINT128, helper.PadRight(data, 16)...)
	} else if len(data) <= 32 {
		sb.Emit(PUSHINT256, helper.PadRight(data, 32)...)
	} else {
		sb.addError(fmt.Errorf("argument out of range: number"))
	}
}

// new design, support all integer types
func (sb *ScriptBuilder) EmitPushInteger(num interface{}) {
	switch num.(type) {
	case int8:
		sb.EmitPushBigInt(big.NewInt(int64(num.(int8))))
		break
	case uint8:
		sb.EmitPushBigInt(big.NewInt(int64(num.(uint8))))
		break
	case int16:
		sb.EmitPushBigInt(big.NewInt(int64(num.(int16))))
		break
	case uint16:
		sb.EmitPushBigInt(big.NewInt(int64(num.(uint16))))
		break
	case int32:
		sb.EmitPushBigInt(big.NewInt(int64(num.(int32))))
		break
	case uint32:
		sb.EmitPushBigInt(big.NewInt(int64(num.(uint32))))
		break
	case int64:
		sb.EmitPushBigInt(big.NewInt(num.(int64)))
		break
	case uint64:
		sb.EmitPushBigInt(big.NewInt(int64(num.(uint64))))
		break
	case int:
		sb.EmitPushBigInt(big.NewInt(int64(num.(int))))
		break
	case uint:
		sb.EmitPushBigInt(big.NewInt(int64(num.(uint))))
		break
	default:
		sb.addError(fmt.Errorf("param is not of integer type"))
	}
}

func (sb *ScriptBuilder) EmitPushBool(data bool) {
	if data {
		sb.Emit(PUSH1)
	} else {
		sb.Emit(PUSH0)
	}
}

func (sb *ScriptBuilder) EmitPushBytes(data []byte) {
	if data == nil {
		sb.addError(fmt.Errorf("data is empty"))
		return
	}
	l := len(data)
	if l < int(0x100) {
		sb.Emit(PUSHDATA1)
		sb.buff.WriteByte(byte(l))
		sb.buff.Write(data)
	} else if l < int(0x10000) {
		sb.Emit(PUSHDATA2)
		sb.buff.Write(helper.UInt16ToBytes(uint16(l)))
		sb.buff.Write(data)
	} else {
		sb.Emit(PUSHDATA4)
		sb.buff.Write(helper.UInt32ToBytes(uint32(l)))
		sb.buff.Write(data)
	}
}

// Convert the string to UTF8 format encoded byte array
func (sb *ScriptBuilder) EmitPushString(data string) {
	sb.EmitPushBytes([]byte(data))
}

// EmitRaw pushes a raw byte array
func (sb *ScriptBuilder) EmitRaw(arg []byte) {
	if arg != nil {
		sb.buff.Write(arg)
	}
}

func (sb *ScriptBuilder) EmitSysCall(api uint) {
	sb.Emit(SYSCALL, helper.UInt32ToBytes(uint32(api))...)
}

// below methods are from the extension helper in VM.Helper.cs

func (sb *ScriptBuilder) CreateArray(list []interface{}) {
	if list == nil || len(list) == 0 {
		sb.Emit(NEWARRAY0)
		return
	}
	for i := len(list) - 1; i >= 0; i-- {
		sb.EmitPushObject(list[i])
	}
	sb.EmitPushInteger(len(list))
	sb.Emit(PACK)
}

func (sb *ScriptBuilder) CreateMap(m map[interface{}]interface{}) {
	sb.Emit(NEWMAP)
	if m != nil {
		for k, v := range m {
			sb.Emit(DUP)
			sb.EmitPushObject(k)
			sb.EmitPushObject(v)
			sb.Emit(SETITEM)
		}
	}
}

func (sb *ScriptBuilder) EmitOpCodes(ops ...OpCode) {
	if ops == nil {
		return
	}
	for _, op := range ops {
		sb.Emit(op)
	}
}

func (sb *ScriptBuilder) EmitDynamicCall(scriptHash *helper.UInt160, operation string) {
	sb.Emit(NEWARRAY0)
	sb.EmitPushObject(All)
	sb.EmitPushString(operation)
	sb.EmitPushSerializable(scriptHash)
	sb.EmitSysCall(Call.ToInteropMethodHash())
}

func (sb *ScriptBuilder) EmitDynamicCallParam(scriptHash *helper.UInt160, operation string, args ...ContractParameter) {
	for i := len(args) - 1; i >= 0; i-- {
		sb.EmitPushParameter(args[i])
	}
	sb.EmitPushInteger(len(args))
	sb.Emit(PACK)
	sb.EmitPushObject(All)
	sb.EmitPushString(operation)
	sb.EmitPushSerializable(scriptHash)
	sb.EmitSysCall(Call.ToInteropMethodHash())
}

func (sb *ScriptBuilder) EmitDynamicCallObj(scriptHash *helper.UInt160, operation string, objs []interface{}) {
	for i := len(objs) - 1; i >= 0; i-- {
		sb.EmitPushObject(objs[i])
	}
	sb.EmitPushInteger(len(objs))
	sb.Emit(PACK)
	sb.EmitPushObject(All)
	sb.EmitPushString(operation)
	sb.EmitPushSerializable(scriptHash)
	sb.EmitSysCall(Call.ToInteropMethodHash())
}

func (sb *ScriptBuilder) EmitPushSerializable(data io.ISerializable) {
	b, e := io.ToArray(data)
	sb.addError(e)
	sb.EmitPushBytes(b)
}

func (sb *ScriptBuilder) EmitPushParameter(param ContractParameter) {
	if param.Value == nil {
		sb.Emit(PUSHNULL)
		return
	}
	switch param.Type {
	case Signature, ByteArray:
		sb.EmitPushBytes(param.Value.([]byte))
		break
	case Boolean:
		sb.EmitPushBool(param.Value.(bool))
		break
	case Integer:
		sb.EmitPushObject(param.Value)
		break
	case Hash160:
		sb.EmitPushSerializable(param.Value.(*helper.UInt160))
		break
	case Hash256:
		sb.EmitPushSerializable(param.Value.(*helper.UInt256))
		break
	case PublicKey:
		sb.EmitPushBytes(param.Value.([]byte)) // import cycle not allowed
		break
	case String:
		sb.EmitPushString(param.Value.(string))
		break
	case Array:
		a := param.Value.([]ContractParameter)
		for i := len(a) - 1; i >= 0; i-- {
			sb.EmitPushParameter(a[i])
		}
		sb.EmitPushInteger(len(a))
		sb.Emit(PACK)
		break
	case Map:
		pairs := param.Value.(map[interface{}]interface{})
		sb.CreateMap(pairs)
		break
	default:
		sb.addError(fmt.Errorf("invalid param type"))
		break
	}
}

func (sb *ScriptBuilder) EmitPushObject(obj interface{}) {
	switch obj.(type) {
	case CallFlags:
		sb.EmitPushBigInt(big.NewInt(int64(obj.(CallFlags))))
		break
	case bool:
		sb.EmitPushBool(obj.(bool))
		break
	case []byte:
		sb.EmitPushBytes(obj.([]byte))
		break
	case string:
		sb.EmitPushString(obj.(string))
		break
	case big.Int:
		bi := obj.(big.Int)
		sb.EmitPushBigInt(&bi)
		break
	case *big.Int:
		sb.EmitPushBigInt(obj.(*big.Int))
		break
	case io.ISerializable:
		sb.EmitPushSerializable(obj.(io.ISerializable))
		break
	case int8, uint8, int16, uint16,
	     int32, uint32, int64, uint64,
	     int, uint:
		sb.EmitPushInteger(obj)
		break
	case ContractParameter:
		sb.EmitPushParameter(obj.(ContractParameter))
		break
	case types.Nil:
		sb.Emit(PUSHNULL)
		break
	default:
		sb.addError(fmt.Errorf("invalid argument type"))
		break
	}
}

func (sb *ScriptBuilder) EmitSysCallWithParam(method uint, args ...interface{}) {
	if args != nil {
		l := len(args)
		for i := l - 1; i >= 0; i-- {
			sb.EmitPushObject(args[i])
		}
	}
	sb.EmitSysCall(method)
}

// Generate scripts to call a specific method from a specific contract.
func MakeScript(scriptHash *helper.UInt160, operation string, objs []interface{}) ([]byte, error) {
	sb := NewScriptBuilder()
	if objs != nil && len(objs) > 0 {
		sb.EmitDynamicCallObj(scriptHash, operation, objs)
	} else {
		sb.EmitDynamicCall(scriptHash, operation)
	}
	return sb.ToArray()
}
