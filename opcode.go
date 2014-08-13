// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"hash"
	"math/big"

	"code.google.com/p/go.crypto/ripemd160"
	"github.com/conformal/btcec"
	"github.com/conformal/btcwire"
	"github.com/conformal/fastsha256"
	"github.com/davecgh/go-spew/spew"
)

// An opcode defines the information related to a btcscript opcode.
// opfunc if present is the function to call to perform the opcode on
// the script. The current script is passed in as a slice with the firs
// member being the opcode itself.
type opcode struct {
	value     byte
	name      string
	length    int
	opfunc    func(*parsedOpcode, *Script) error
	parsefunc func(*opcode, *Script, []byte) error
}

// These constants are the values of the official opcode used on the btc wiki,
// in bitcoind and in most if not all other references and software related to
// handling BTC scripts.
const (
	OP_FALSE               = 0 // AKA OP_0
	OP_0                   = 0
	OP_DATA_1              = 1
	OP_DATA_2              = 2
	OP_DATA_3              = 3
	OP_DATA_4              = 4
	OP_DATA_5              = 5
	OP_DATA_6              = 6
	OP_DATA_7              = 7
	OP_DATA_8              = 8
	OP_DATA_9              = 9
	OP_DATA_10             = 10
	OP_DATA_11             = 11
	OP_DATA_12             = 12
	OP_DATA_13             = 13
	OP_DATA_14             = 14
	OP_DATA_15             = 15
	OP_DATA_16             = 16
	OP_DATA_17             = 17
	OP_DATA_18             = 18
	OP_DATA_19             = 19
	OP_DATA_20             = 20
	OP_DATA_21             = 21
	OP_DATA_22             = 22
	OP_DATA_23             = 23
	OP_DATA_24             = 24
	OP_DATA_25             = 25
	OP_DATA_26             = 26
	OP_DATA_27             = 27
	OP_DATA_28             = 28
	OP_DATA_29             = 29
	OP_DATA_30             = 30
	OP_DATA_31             = 31
	OP_DATA_32             = 32
	OP_DATA_33             = 33
	OP_DATA_34             = 34
	OP_DATA_35             = 35
	OP_DATA_36             = 36
	OP_DATA_37             = 37
	OP_DATA_38             = 38
	OP_DATA_39             = 39
	OP_DATA_40             = 40
	OP_DATA_41             = 41
	OP_DATA_42             = 42
	OP_DATA_43             = 43
	OP_DATA_44             = 44
	OP_DATA_45             = 45
	OP_DATA_46             = 46
	OP_DATA_47             = 47
	OP_DATA_48             = 48
	OP_DATA_49             = 49
	OP_DATA_50             = 50
	OP_DATA_51             = 51
	OP_DATA_52             = 52
	OP_DATA_53             = 53
	OP_DATA_54             = 54
	OP_DATA_55             = 55
	OP_DATA_56             = 56
	OP_DATA_57             = 57
	OP_DATA_58             = 58
	OP_DATA_59             = 59
	OP_DATA_60             = 60
	OP_DATA_61             = 61
	OP_DATA_62             = 62
	OP_DATA_63             = 63
	OP_DATA_64             = 64
	OP_DATA_65             = 65
	OP_DATA_66             = 66
	OP_DATA_67             = 67
	OP_DATA_68             = 68
	OP_DATA_69             = 69
	OP_DATA_70             = 70
	OP_DATA_71             = 71
	OP_DATA_72             = 72
	OP_DATA_73             = 73
	OP_DATA_74             = 74
	OP_DATA_75             = 75
	OP_PUSHDATA1           = 76
	OP_PUSHDATA2           = 77
	OP_PUSHDATA4           = 78
	OP_1NEGATE             = 79
	OP_RESERVED            = 80
	OP_1                   = 81 // AKA OP_TRUE
	OP_TRUE                = 81
	OP_2                   = 82
	OP_3                   = 83
	OP_4                   = 84
	OP_5                   = 85
	OP_6                   = 86
	OP_7                   = 87
	OP_8                   = 88
	OP_9                   = 89
	OP_10                  = 90
	OP_11                  = 91
	OP_12                  = 92
	OP_13                  = 93
	OP_14                  = 94
	OP_15                  = 95
	OP_16                  = 96
	OP_NOP                 = 97
	OP_VER                 = 98
	OP_IF                  = 99
	OP_NOTIF               = 100
	OP_VERIF               = 101
	OP_VERNOTIF            = 102
	OP_ELSE                = 103
	OP_ENDIF               = 104
	OP_VERIFY              = 105
	OP_RETURN              = 106
	OP_TOALTSTACK          = 107
	OP_FROMALTSTACK        = 108
	OP_2DROP               = 109
	OP_2DUP                = 110
	OP_3DUP                = 111
	OP_2OVER               = 112
	OP_2ROT                = 113
	OP_2SWAP               = 114
	OP_IFDUP               = 115
	OP_DEPTH               = 116
	OP_DROP                = 117
	OP_DUP                 = 118
	OP_NIP                 = 119
	OP_OVER                = 120
	OP_PICK                = 121
	OP_ROLL                = 122
	OP_ROT                 = 123
	OP_SWAP                = 124
	OP_TUCK                = 125
	OP_CAT                 = 126
	OP_SUBSTR              = 127
	OP_LEFT                = 128
	OP_RIGHT               = 129
	OP_SIZE                = 130
	OP_INVERT              = 131
	OP_AND                 = 132
	OP_OR                  = 133
	OP_XOR                 = 134
	OP_EQUAL               = 135
	OP_EQUALVERIFY         = 136
	OP_RESERVED1           = 137
	OP_RESERVED2           = 138
	OP_1ADD                = 139
	OP_1SUB                = 140
	OP_2MUL                = 141
	OP_2DIV                = 142
	OP_NEGATE              = 143
	OP_ABS                 = 144
	OP_NOT                 = 145
	OP_0NOTEQUAL           = 146
	OP_ADD                 = 147
	OP_SUB                 = 148
	OP_MUL                 = 149
	OP_DIV                 = 150
	OP_MOD                 = 151
	OP_LSHIFT              = 152
	OP_RSHIFT              = 153
	OP_BOOLAND             = 154
	OP_BOOLOR              = 155
	OP_NUMEQUAL            = 156
	OP_NUMEQUALVERIFY      = 157
	OP_NUMNOTEQUAL         = 158
	OP_LESSTHAN            = 159
	OP_GREATERTHAN         = 160
	OP_LESSTHANOREQUAL     = 161
	OP_GREATERTHANOREQUAL  = 162
	OP_MIN                 = 163
	OP_MAX                 = 164
	OP_WITHIN              = 165
	OP_RIPEMD160           = 166
	OP_SHA1                = 167
	OP_SHA256              = 168
	OP_HASH160             = 169
	OP_HASH256             = 170
	OP_CODESEPARATOR       = 171
	OP_CHECKSIG            = 172
	OP_CHECKSIGVERIFY      = 173
	OP_CHECKMULTISIG       = 174
	OP_CHECKMULTISIGVERIFY = 175
	OP_NOP1                = 176
	OP_NOP2                = 177
	OP_NOP3                = 178
	OP_NOP4                = 179
	OP_NOP5                = 180
	OP_NOP6                = 181
	OP_NOP7                = 182
	OP_NOP8                = 183
	OP_NOP9                = 184
	OP_NOP10               = 185
	OP_UNKNOWN186          = 186
	OP_UNKNOWN187          = 187
	OP_UNKNOWN188          = 188
	OP_UNKNOWN189          = 189
	OP_UNKNOWN190          = 190
	OP_UNKNOWN191          = 191
	OP_UNKNOWN192          = 192
	OP_UNKNOWN193          = 193
	OP_UNKNOWN194          = 194
	OP_UNKNOWN195          = 195
	OP_UNKNOWN196          = 196
	OP_UNKNOWN197          = 197
	OP_UNKNOWN198          = 198
	OP_UNKNOWN199          = 199
	OP_UNKNOWN200          = 200
	OP_UNKNOWN201          = 201
	OP_UNKNOWN202          = 202
	OP_UNKNOWN203          = 203
	OP_UNKNOWN204          = 204
	OP_UNKNOWN205          = 205
	OP_UNKNOWN206          = 206
	OP_UNKNOWN207          = 207
	OP_UNKNOWN208          = 208
	OP_UNKNOWN209          = 209
	OP_UNKNOWN210          = 210
	OP_UNKNOWN211          = 211
	OP_UNKNOWN212          = 212
	OP_UNKNOWN213          = 213
	OP_UNKNOWN214          = 214
	OP_UNKNOWN215          = 215
	OP_UNKNOWN216          = 216
	OP_UNKNOWN217          = 217
	OP_UNKNOWN218          = 218
	OP_UNKNOWN219          = 219
	OP_UNKNOWN220          = 220
	OP_UNKNOWN221          = 221
	OP_UNKNOWN222          = 222
	OP_UNKNOWN223          = 223
	OP_UNKNOWN224          = 224
	OP_UNKNOWN225          = 225
	OP_UNKNOWN226          = 226
	OP_UNKNOWN227          = 227
	OP_UNKNOWN228          = 228
	OP_UNKNOWN229          = 229
	OP_UNKNOWN230          = 230
	OP_UNKNOWN231          = 231
	OP_UNKNOWN232          = 232
	OP_UNKNOWN233          = 233
	OP_UNKNOWN234          = 234
	OP_UNKNOWN235          = 235
	OP_UNKNOWN236          = 236
	OP_UNKNOWN237          = 237
	OP_UNKNOWN238          = 238
	OP_UNKNOWN239          = 239
	OP_UNKNOWN240          = 240
	OP_UNKNOWN241          = 241
	OP_UNKNOWN242          = 242
	OP_UNKNOWN243          = 243
	OP_UNKNOWN244          = 244
	OP_UNKNOWN245          = 245
	OP_UNKNOWN246          = 246
	OP_UNKNOWN247          = 247
	OP_UNKNOWN248          = 248
	OP_UNKNOWN249          = 249
	OP_UNKNOWN250          = 250
	OP_UNKNOWN251          = 251
	OP_UNKNOWN252          = 252
	OP_PUBKEYHASH          = 253 // bitcoind internal, for completeness
	OP_PUBKEY              = 254 // bitcoind internal, for completeness
	OP_INVALIDOPCODE       = 255 // bitcoind internal, for completeness
)

// conditional execution constants
const (
	OpCondFalse = 0
	OpCondTrue  = 1
	OpCondSkip  = 2
)

// Some of the functions in opcodemap call things that themselves then will
// reference the opcodemap to make decisions (things like op_checksig which
// needs to parse scripts to remove opcodes, for example).
// The go compiler is very conservative in this matter and will think there
// is an initialisation loop. In order to work around this we have the fake
// ``prevariable'' opcodemapPreinit and then set the real variable to the
// preinit in init()
var opcodemap map[byte]*opcode

func init() {
	opcodemap = opcodemapPreinit
}

var opcodemapPreinit = map[byte]*opcode{
	OP_FALSE: {value: OP_FALSE, name: "OP_0", length: 1,
		opfunc: opcodeFalse},
	OP_DATA_1: {value: OP_DATA_1, name: "OP_DATA_1", length: 2,
		opfunc: opcodePushData},
	OP_DATA_2: {value: OP_DATA_2, name: "OP_DATA_2", length: 3,
		opfunc: opcodePushData},
	OP_DATA_3: {value: OP_DATA_3, name: "OP_DATA_3", length: 4,
		opfunc: opcodePushData},
	OP_DATA_4: {value: OP_DATA_4, name: "OP_DATA_4", length: 5,
		opfunc: opcodePushData},
	OP_DATA_5: {value: OP_DATA_5, name: "OP_DATA_5", length: 6,
		opfunc: opcodePushData},
	OP_DATA_6: {value: OP_DATA_6, name: "OP_DATA_6", length: 7,
		opfunc: opcodePushData},
	OP_DATA_7: {value: OP_DATA_7, name: "OP_DATA_7", length: 8,
		opfunc: opcodePushData},
	OP_DATA_8: {value: OP_DATA_8, name: "OP_DATA_8", length: 9,
		opfunc: opcodePushData},
	OP_DATA_9: {value: OP_DATA_9, name: "OP_DATA_9", length: 10,
		opfunc: opcodePushData},
	OP_DATA_10: {value: OP_DATA_10, name: "OP_DATA_10", length: 11,
		opfunc: opcodePushData},
	OP_DATA_11: {value: OP_DATA_11, name: "OP_DATA_11", length: 12,
		opfunc: opcodePushData},
	OP_DATA_12: {value: OP_DATA_12, name: "OP_DATA_12", length: 13,
		opfunc: opcodePushData},
	OP_DATA_13: {value: OP_DATA_13, name: "OP_DATA_13", length: 14,
		opfunc: opcodePushData},
	OP_DATA_14: {value: OP_DATA_14, name: "OP_DATA_14", length: 15,
		opfunc: opcodePushData},
	OP_DATA_15: {value: OP_DATA_15, name: "OP_DATA_15", length: 16,
		opfunc: opcodePushData},
	OP_DATA_16: {value: OP_DATA_16, name: "OP_DATA_16", length: 17,
		opfunc: opcodePushData},
	OP_DATA_17: {value: OP_DATA_17, name: "OP_DATA_17", length: 18,
		opfunc: opcodePushData},
	OP_DATA_18: {value: OP_DATA_18, name: "OP_DATA_18", length: 19,
		opfunc: opcodePushData},
	OP_DATA_19: {value: OP_DATA_19, name: "OP_DATA_19", length: 20,
		opfunc: opcodePushData},
	OP_DATA_20: {value: OP_DATA_20, name: "OP_DATA_20", length: 21,
		opfunc: opcodePushData},
	OP_DATA_21: {value: OP_DATA_21, name: "OP_DATA_21", length: 22,
		opfunc: opcodePushData},
	OP_DATA_22: {value: OP_DATA_22, name: "OP_DATA_22", length: 23,
		opfunc: opcodePushData},
	OP_DATA_23: {value: OP_DATA_23, name: "OP_DATA_23", length: 24,
		opfunc: opcodePushData},
	OP_DATA_24: {value: OP_DATA_24, name: "OP_DATA_24", length: 25,
		opfunc: opcodePushData},
	OP_DATA_25: {value: OP_DATA_25, name: "OP_DATA_25", length: 26,
		opfunc: opcodePushData},
	OP_DATA_26: {value: OP_DATA_26, name: "OP_DATA_26", length: 27,
		opfunc: opcodePushData},
	OP_DATA_27: {value: OP_DATA_27, name: "OP_DATA_27", length: 28,
		opfunc: opcodePushData},
	OP_DATA_28: {value: OP_DATA_28, name: "OP_DATA_28", length: 29,
		opfunc: opcodePushData},
	OP_DATA_29: {value: OP_DATA_29, name: "OP_DATA_29", length: 30,
		opfunc: opcodePushData},
	OP_DATA_30: {value: OP_DATA_30, name: "OP_DATA_30", length: 31,
		opfunc: opcodePushData},
	OP_DATA_31: {value: OP_DATA_31, name: "OP_DATA_31", length: 32,
		opfunc: opcodePushData},
	OP_DATA_32: {value: OP_DATA_32, name: "OP_DATA_32", length: 33,
		opfunc: opcodePushData},
	OP_DATA_33: {value: OP_DATA_33, name: "OP_DATA_33", length: 34,
		opfunc: opcodePushData},
	OP_DATA_34: {value: OP_DATA_34, name: "OP_DATA_34", length: 35,
		opfunc: opcodePushData},
	OP_DATA_35: {value: OP_DATA_35, name: "OP_DATA_35", length: 36,
		opfunc: opcodePushData},
	OP_DATA_36: {value: OP_DATA_36, name: "OP_DATA_36", length: 37,
		opfunc: opcodePushData},
	OP_DATA_37: {value: OP_DATA_37, name: "OP_DATA_37", length: 38,
		opfunc: opcodePushData},
	OP_DATA_38: {value: OP_DATA_38, name: "OP_DATA_38", length: 39,
		opfunc: opcodePushData},
	OP_DATA_39: {value: OP_DATA_39, name: "OP_DATA_39", length: 40,
		opfunc: opcodePushData},
	OP_DATA_40: {value: OP_DATA_40, name: "OP_DATA_40", length: 41,
		opfunc: opcodePushData},
	OP_DATA_41: {value: OP_DATA_41, name: "OP_DATA_41", length: 42,
		opfunc: opcodePushData},
	OP_DATA_42: {value: OP_DATA_42, name: "OP_DATA_42", length: 43,
		opfunc: opcodePushData},
	OP_DATA_43: {value: OP_DATA_43, name: "OP_DATA_43", length: 44,
		opfunc: opcodePushData},
	OP_DATA_44: {value: OP_DATA_44, name: "OP_DATA_44", length: 45,
		opfunc: opcodePushData},
	OP_DATA_45: {value: OP_DATA_45, name: "OP_DATA_45", length: 46,
		opfunc: opcodePushData},
	OP_DATA_46: {value: OP_DATA_46, name: "OP_DATA_46", length: 47,
		opfunc: opcodePushData},
	OP_DATA_47: {value: OP_DATA_47, name: "OP_DATA_47", length: 48,
		opfunc: opcodePushData},
	OP_DATA_48: {value: OP_DATA_48, name: "OP_DATA_48", length: 49,
		opfunc: opcodePushData},
	OP_DATA_49: {value: OP_DATA_49, name: "OP_DATA_49", length: 50,
		opfunc: opcodePushData},
	OP_DATA_50: {value: OP_DATA_50, name: "OP_DATA_50", length: 51,
		opfunc: opcodePushData},
	OP_DATA_51: {value: OP_DATA_51, name: "OP_DATA_51", length: 52,
		opfunc: opcodePushData},
	OP_DATA_52: {value: OP_DATA_52, name: "OP_DATA_52", length: 53,
		opfunc: opcodePushData},
	OP_DATA_53: {value: OP_DATA_53, name: "OP_DATA_53", length: 54,
		opfunc: opcodePushData},
	OP_DATA_54: {value: OP_DATA_54, name: "OP_DATA_54", length: 55,
		opfunc: opcodePushData},
	OP_DATA_55: {value: OP_DATA_55, name: "OP_DATA_55", length: 56,
		opfunc: opcodePushData},
	OP_DATA_56: {value: OP_DATA_56, name: "OP_DATA_56", length: 57,
		opfunc: opcodePushData},
	OP_DATA_57: {value: OP_DATA_57, name: "OP_DATA_57", length: 58,
		opfunc: opcodePushData},
	OP_DATA_58: {value: OP_DATA_58, name: "OP_DATA_58", length: 59,
		opfunc: opcodePushData},
	OP_DATA_59: {value: OP_DATA_59, name: "OP_DATA_59", length: 60,
		opfunc: opcodePushData},
	OP_DATA_60: {value: OP_DATA_60, name: "OP_DATA_60", length: 61,
		opfunc: opcodePushData},
	OP_DATA_61: {value: OP_DATA_61, name: "OP_DATA_61", length: 62,
		opfunc: opcodePushData},
	OP_DATA_62: {value: OP_DATA_62, name: "OP_DATA_62", length: 63,
		opfunc: opcodePushData},
	OP_DATA_63: {value: OP_DATA_63, name: "OP_DATA_63", length: 64,
		opfunc: opcodePushData},
	OP_DATA_64: {value: OP_DATA_64, name: "OP_DATA_64", length: 65,
		opfunc: opcodePushData},
	OP_DATA_65: {value: OP_DATA_65, name: "OP_DATA_65", length: 66,
		opfunc: opcodePushData},
	OP_DATA_66: {value: OP_DATA_66, name: "OP_DATA_66", length: 67,
		opfunc: opcodePushData},
	OP_DATA_67: {value: OP_DATA_67, name: "OP_DATA_67", length: 68,
		opfunc: opcodePushData},
	OP_DATA_68: {value: OP_DATA_68, name: "OP_DATA_68", length: 69,
		opfunc: opcodePushData},
	OP_DATA_69: {value: OP_DATA_69, name: "OP_DATA_69", length: 70,
		opfunc: opcodePushData},
	OP_DATA_70: {value: OP_DATA_70, name: "OP_DATA_70", length: 71,
		opfunc: opcodePushData},
	OP_DATA_71: {value: OP_DATA_71, name: "OP_DATA_71", length: 72,
		opfunc: opcodePushData},
	OP_DATA_72: {value: OP_DATA_72, name: "OP_DATA_72", length: 73,
		opfunc: opcodePushData},
	OP_DATA_73: {value: OP_DATA_73, name: "OP_DATA_73", length: 74,
		opfunc: opcodePushData},
	OP_DATA_74: {value: OP_DATA_74, name: "OP_DATA_74", length: 75,
		opfunc: opcodePushData},
	OP_DATA_75: {value: OP_DATA_75, name: "OP_DATA_75", length: 76,
		opfunc: opcodePushData},
	OP_PUSHDATA1: {value: OP_PUSHDATA1, name: "OP_PUSHDATA1", length: -1,
		opfunc: opcodePushData},
	OP_PUSHDATA2: {value: OP_PUSHDATA2, name: "OP_PUSHDATA2", length: -2,
		opfunc: opcodePushData},
	OP_PUSHDATA4: {value: OP_PUSHDATA4, name: "OP_PUSHDATA4", length: -4,
		opfunc: opcodePushData},
	OP_1NEGATE: {value: OP_1NEGATE, name: "OP_1NEGATE", length: 1,
		opfunc: opcode1Negate},
	OP_RESERVED: {value: OP_RESERVED, name: "OP_RESERVED", length: 1,
		opfunc: opcodeReserved},
	OP_TRUE: {value: OP_TRUE, name: "OP_1", length: 1,
		opfunc: opcodeN},
	OP_2: {value: OP_2, name: "OP_2", length: 1,
		opfunc: opcodeN},
	OP_3: {value: OP_3, name: "OP_3", length: 1,
		opfunc: opcodeN},
	OP_4: {value: OP_4, name: "OP_4", length: 1,
		opfunc: opcodeN},
	OP_5: {value: OP_5, name: "OP_5", length: 1,
		opfunc: opcodeN},
	OP_6: {value: OP_6, name: "OP_6", length: 1,
		opfunc: opcodeN},
	OP_7: {value: OP_7, name: "OP_7", length: 1,
		opfunc: opcodeN},
	OP_8: {value: OP_8, name: "OP_8", length: 1,
		opfunc: opcodeN},
	OP_9: {value: OP_9, name: "OP_9", length: 1,
		opfunc: opcodeN},
	OP_10: {value: OP_10, name: "OP_10", length: 1,
		opfunc: opcodeN},
	OP_11: {value: OP_11, name: "OP_11", length: 1,
		opfunc: opcodeN},
	OP_12: {value: OP_12, name: "OP_12", length: 1,
		opfunc: opcodeN},
	OP_13: {value: OP_13, name: "OP_13", length: 1,
		opfunc: opcodeN},
	OP_14: {value: OP_14, name: "OP_14", length: 1,
		opfunc: opcodeN},
	OP_15: {value: OP_15, name: "OP_15", length: 1,
		opfunc: opcodeN},
	OP_16: {value: OP_16, name: "OP_16", length: 1,
		opfunc: opcodeN},
	OP_NOP: {value: OP_NOP, name: "OP_NOP", length: 1,
		opfunc: opcodeNop},
	OP_VER: {value: OP_VER, name: "OP_VER", length: 1,
		opfunc: opcodeReserved},
	OP_IF: {value: OP_IF, name: "OP_IF", length: 1,
		opfunc: opcodeIf},
	OP_NOTIF: {value: OP_NOTIF, name: "OP_NOTIF", length: 1,
		opfunc: opcodeNotIf},
	OP_VERIF: {value: OP_VERIF, name: "OP_VERIF", length: 1,
		opfunc: opcodeReserved},
	OP_VERNOTIF: {value: OP_VERNOTIF, name: "OP_VERNOTIF", length: 1,
		opfunc: opcodeReserved},
	OP_ELSE: {value: OP_ELSE, name: "OP_ELSE", length: 1,
		opfunc: opcodeElse},
	OP_ENDIF: {value: OP_ENDIF, name: "OP_ENDIF", length: 1,
		opfunc: opcodeEndif},
	OP_VERIFY: {value: OP_VERIFY, name: "OP_VERIFY", length: 1,
		opfunc: opcodeVerify},
	OP_RETURN: {value: OP_RETURN, name: "OP_RETURN", length: 1,
		opfunc: opcodeReturn},
	OP_TOALTSTACK: {value: OP_TOALTSTACK, name: "OP_TOALTSTACK", length: 1,
		opfunc: opcodeToAltStack},
	OP_FROMALTSTACK: {value: OP_FROMALTSTACK, name: "OP_FROMALTSTACK", length: 1,
		opfunc: opcodeFromAltStack},
	OP_2DROP: {value: OP_2DROP, name: "OP_2DROP", length: 1,
		opfunc: opcode2Drop},
	OP_2DUP: {value: OP_2DUP, name: "OP_2DUP", length: 1,
		opfunc: opcode2Dup},
	OP_3DUP: {value: OP_3DUP, name: "OP_3DUP", length: 1,
		opfunc: opcode3Dup},
	OP_2OVER: {value: OP_2OVER, name: "OP_2OVER", length: 1,
		opfunc: opcode2Over},
	OP_2ROT: {value: OP_2ROT, name: "OP_2ROT", length: 1,
		opfunc: opcode2Rot},
	OP_2SWAP: {value: OP_2SWAP, name: "OP_2SWAP", length: 1,
		opfunc: opcode2Swap},
	OP_IFDUP: {value: OP_IFDUP, name: "OP_IFDUP", length: 1,
		opfunc: opcodeIfDup},
	OP_DEPTH: {value: OP_DEPTH, name: "OP_DEPTH", length: 1,
		opfunc: opcodeDepth},
	OP_DROP: {value: OP_DROP, name: "OP_DROP", length: 1,
		opfunc: opcodeDrop},
	OP_DUP: {value: OP_DUP, name: "OP_DUP", length: 1,
		opfunc: opcodeDup},
	OP_NIP: {value: OP_NIP, name: "OP_NIP", length: 1,
		opfunc: opcodeNip},
	OP_OVER: {value: OP_OVER, name: "OP_OVER", length: 1,
		opfunc: opcodeOver},
	OP_PICK: {value: OP_PICK, name: "OP_PICK", length: 1,
		opfunc: opcodePick},
	OP_ROLL: {value: OP_ROLL, name: "OP_ROLL", length: 1,
		opfunc: opcodeRoll},
	OP_ROT: {value: OP_ROT, name: "OP_ROT", length: 1,
		opfunc: opcodeRot},
	OP_SWAP: {value: OP_SWAP, name: "OP_SWAP", length: 1,
		opfunc: opcodeSwap},
	OP_TUCK: {value: OP_TUCK, name: "OP_TUCK", length: 1,
		opfunc: opcodeTuck},
	OP_CAT: {value: OP_CAT, name: "OP_CAT", length: 1,
		opfunc: opcodeDisabled},
	OP_SUBSTR: {value: OP_SUBSTR, name: "OP_SUBSTR", length: 1,
		opfunc: opcodeDisabled},
	OP_LEFT: {value: OP_LEFT, name: "OP_LEFT", length: 1,
		opfunc: opcodeDisabled},
	OP_RIGHT: {value: OP_RIGHT, name: "OP_RIGHT", length: 1,
		opfunc: opcodeDisabled},
	OP_SIZE: {value: OP_SIZE, name: "OP_SIZE", length: 1,
		opfunc: opcodeSize},
	OP_INVERT: {value: OP_INVERT, name: "OP_INVERT", length: 1,
		opfunc: opcodeDisabled},
	OP_AND: {value: OP_AND, name: "OP_AND", length: 1,
		opfunc: opcodeDisabled},
	OP_OR: {value: OP_OR, name: "OP_OR", length: 1,
		opfunc: opcodeDisabled},
	OP_XOR: {value: OP_XOR, name: "OP_XOR", length: 1,
		opfunc: opcodeDisabled},
	OP_EQUAL: {value: OP_EQUAL, name: "OP_EQUAL", length: 1,
		opfunc: opcodeEqual},
	OP_EQUALVERIFY: {value: OP_EQUALVERIFY, name: "OP_EQUALVERIFY", length: 1,
		opfunc: opcodeEqualVerify},
	OP_RESERVED1: {value: OP_RESERVED1, name: "OP_RESERVED1", length: 1,
		opfunc: opcodeReserved},
	OP_RESERVED2: {value: OP_RESERVED2, name: "OP_RESERVED2", length: 1,
		opfunc: opcodeReserved},
	OP_1ADD: {value: OP_1ADD, name: "OP_1ADD", length: 1,
		opfunc: opcode1Add},
	OP_1SUB: {value: OP_1SUB, name: "OP_1SUB", length: 1,
		opfunc: opcode1Sub},
	OP_2MUL: {value: OP_2MUL, name: "OP_2MUL", length: 1,
		opfunc: opcodeDisabled},
	OP_2DIV: {value: OP_2DIV, name: "OP_2DIV", length: 1,
		opfunc: opcodeDisabled},
	OP_NEGATE: {value: OP_NEGATE, name: "OP_NEGATE", length: 1,
		opfunc: opcodeNegate},
	OP_ABS: {value: OP_ABS, name: "OP_ABS", length: 1,
		opfunc: opcodeAbs},
	OP_NOT: {value: OP_NOT, name: "OP_NOT", length: 1,
		opfunc: opcodeNot},
	OP_0NOTEQUAL: {value: OP_0NOTEQUAL, name: "OP_0NOTEQUAL", length: 1,
		opfunc: opcode0NotEqual},
	OP_ADD: {value: OP_ADD, name: "OP_ADD", length: 1,
		opfunc: opcodeAdd},
	OP_SUB: {value: OP_SUB, name: "OP_SUB", length: 1,
		opfunc: opcodeSub},
	OP_MUL: {value: OP_MUL, name: "OP_MUL", length: 1,
		opfunc: opcodeDisabled},
	OP_DIV: {value: OP_DIV, name: "OP_DIV", length: 1,
		opfunc: opcodeDisabled},
	OP_MOD: {value: OP_MOD, name: "OP_MOD", length: 1,
		opfunc: opcodeDisabled},
	OP_LSHIFT: {value: OP_LSHIFT, name: "OP_LSHIFT", length: 1,
		opfunc: opcodeDisabled},
	OP_RSHIFT: {value: OP_RSHIFT, name: "OP_RSHIFT", length: 1,
		opfunc: opcodeDisabled},
	OP_BOOLAND: {value: OP_BOOLAND, name: "OP_BOOLAND", length: 1,
		opfunc: opcodeBoolAnd},
	OP_BOOLOR: {value: OP_BOOLOR, name: "OP_BOOLOR", length: 1,
		opfunc: opcodeBoolOr},
	OP_NUMEQUAL: {value: OP_NUMEQUAL, name: "OP_NUMEQUAL", length: 1,
		opfunc: opcodeNumEqual},
	OP_NUMEQUALVERIFY: {value: OP_NUMEQUALVERIFY, name: "OP_NUMEQUALVERIFY", length: 1,
		opfunc: opcodeNumEqualVerify},
	OP_NUMNOTEQUAL: {value: OP_NUMNOTEQUAL, name: "OP_NUMNOTEQUAL", length: 1,
		opfunc: opcodeNumNotEqual},
	OP_LESSTHAN: {value: OP_LESSTHAN, name: "OP_LESSTHAN", length: 1,
		opfunc: opcodeLessThan},
	OP_GREATERTHAN: {value: OP_GREATERTHAN, name: "OP_GREATERTHAN", length: 1,
		opfunc: opcodeGreaterThan},
	OP_LESSTHANOREQUAL: {value: OP_LESSTHANOREQUAL, name: "OP_LESSTHANOREQUAL", length: 1,
		opfunc: opcodeLessThanOrEqual},
	OP_GREATERTHANOREQUAL: {value: OP_GREATERTHANOREQUAL, name: "OP_GREATERTHANOREQUAL", length: 1,
		opfunc: opcodeGreaterThanOrEqual},
	OP_MIN: {value: OP_MIN, name: "OP_MIN", length: 1,
		opfunc: opcodeMin},
	OP_MAX: {value: OP_MAX, name: "OP_MAX", length: 1,
		opfunc: opcodeMax},
	OP_WITHIN: {value: OP_WITHIN, name: "OP_WITHIN", length: 1,
		opfunc: opcodeWithin},
	OP_RIPEMD160: {value: OP_RIPEMD160, name: "OP_RIPEMD160", length: 1,
		opfunc: opcodeRipemd160},
	OP_SHA1: {value: OP_SHA1, name: "OP_SHA1", length: 1,
		opfunc: opcodeSha1},
	OP_SHA256: {value: OP_SHA256, name: "OP_SHA256", length: 1,
		opfunc: opcodeSha256},
	OP_HASH160: {value: OP_HASH160, name: "OP_HASH160", length: 1,
		opfunc: opcodeHash160},
	OP_HASH256: {value: OP_HASH256, name: "OP_HASH256", length: 1,
		opfunc: opcodeHash256},
	OP_CODESEPARATOR: {value: OP_CODESEPARATOR, name: "OP_CODESEPARATOR", length: 1,
		opfunc: opcodeCodeSeparator},
	OP_CHECKSIG: {value: OP_CHECKSIG, name: "OP_CHECKSIG", length: 1,
		opfunc: opcodeCheckSig},
	OP_CHECKSIGVERIFY: {value: OP_CHECKSIGVERIFY, name: "OP_CHECKSIGVERIFY", length: 1,
		opfunc: opcodeCheckSigVerify},
	OP_CHECKMULTISIG: {value: OP_CHECKMULTISIG, name: "OP_CHECKMULTISIG", length: 1,
		opfunc: opcodeCheckMultiSig},
	OP_CHECKMULTISIGVERIFY: {value: OP_CHECKMULTISIGVERIFY, name: "OP_CHECKMULTISIGVERIFY", length: 1,

		opfunc: opcodeCheckMultiSigVerify},
	OP_NOP1: {value: OP_NOP1, name: "OP_NOP1", length: 1,
		opfunc: opcodeNop},
	OP_NOP2: {value: OP_NOP2, name: "OP_NOP2", length: 1,
		opfunc: opcodeNop},
	OP_NOP3: {value: OP_NOP3, name: "OP_NOP3", length: 1,
		opfunc: opcodeNop},
	OP_NOP4: {value: OP_NOP4, name: "OP_NOP4", length: 1,
		opfunc: opcodeNop},
	OP_NOP5: {value: OP_NOP5, name: "OP_NOP5", length: 1,
		opfunc: opcodeNop},
	OP_NOP6: {value: OP_NOP6, name: "OP_NOP6", length: 1,
		opfunc: opcodeNop},
	OP_NOP7: {value: OP_NOP7, name: "OP_NOP7", length: 1,
		opfunc: opcodeNop},
	OP_NOP8: {value: OP_NOP8, name: "OP_NOP8", length: 1,
		opfunc: opcodeNop},
	OP_NOP9: {value: OP_NOP9, name: "OP_NOP9", length: 1,
		opfunc: opcodeNop},
	OP_NOP10: {value: OP_NOP10, name: "OP_NOP10", length: 1,
		opfunc: opcodeNop},
	OP_UNKNOWN186: {value: OP_UNKNOWN186, name: "OP_UNKNOWN186", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN187: {value: OP_UNKNOWN187, name: "OP_UNKNOWN187", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN188: {value: OP_UNKNOWN188, name: "OP_UNKNOWN188", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN189: {value: OP_UNKNOWN189, name: "OP_UNKNOWN189", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN190: {value: OP_UNKNOWN190, name: "OP_UNKNOWN190", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN191: {value: OP_UNKNOWN191, name: "OP_UNKNOWN191", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN192: {value: OP_UNKNOWN192, name: "OP_UNKNOWN192", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN193: {value: OP_UNKNOWN193, name: "OP_UNKNOWN193", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN194: {value: OP_UNKNOWN194, name: "OP_UNKNOWN194", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN195: {value: OP_UNKNOWN195, name: "OP_UNKNOWN195", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN196: {value: OP_UNKNOWN196, name: "OP_UNKNOWN196", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN197: {value: OP_UNKNOWN197, name: "OP_UNKNOWN197", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN198: {value: OP_UNKNOWN198, name: "OP_UNKNOWN198", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN199: {value: OP_UNKNOWN199, name: "OP_UNKNOWN199", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN200: {value: OP_UNKNOWN200, name: "OP_UNKNOWN200", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN201: {value: OP_UNKNOWN201, name: "OP_UNKNOWN201", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN202: {value: OP_UNKNOWN202, name: "OP_UNKNOWN202", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN203: {value: OP_UNKNOWN203, name: "OP_UNKNOWN203", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN204: {value: OP_UNKNOWN204, name: "OP_UNKNOWN204", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN205: {value: OP_UNKNOWN205, name: "OP_UNKNOWN205", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN206: {value: OP_UNKNOWN206, name: "OP_UNKNOWN206", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN207: {value: OP_UNKNOWN207, name: "OP_UNKNOWN207", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN208: {value: OP_UNKNOWN208, name: "OP_UNKNOWN208", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN209: {value: OP_UNKNOWN209, name: "OP_UNKNOWN209", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN210: {value: OP_UNKNOWN210, name: "OP_UNKNOWN210", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN211: {value: OP_UNKNOWN211, name: "OP_UNKNOWN211", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN212: {value: OP_UNKNOWN212, name: "OP_UNKNOWN212", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN213: {value: OP_UNKNOWN213, name: "OP_UNKNOWN213", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN214: {value: OP_UNKNOWN214, name: "OP_UNKNOWN214", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN215: {value: OP_UNKNOWN215, name: "OP_UNKNOWN215", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN216: {value: OP_UNKNOWN216, name: "OP_UNKNOWN216", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN217: {value: OP_UNKNOWN217, name: "OP_UNKNOWN217", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN218: {value: OP_UNKNOWN218, name: "OP_UNKNOWN218", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN219: {value: OP_UNKNOWN219, name: "OP_UNKNOWN219", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN220: {value: OP_UNKNOWN220, name: "OP_UNKNOWN220", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN221: {value: OP_UNKNOWN221, name: "OP_UNKNOWN221", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN222: {value: OP_UNKNOWN222, name: "OP_UNKNOWN222", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN223: {value: OP_UNKNOWN223, name: "OP_UNKNOWN223", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN224: {value: OP_UNKNOWN224, name: "OP_UNKNOWN224", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN225: {value: OP_UNKNOWN225, name: "OP_UNKNOWN225", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN226: {value: OP_UNKNOWN226, name: "OP_UNKNOWN226", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN227: {value: OP_UNKNOWN227, name: "OP_UNKNOWN227", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN228: {value: OP_UNKNOWN228, name: "OP_UNKNOWN228", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN229: {value: OP_UNKNOWN229, name: "OP_UNKNOWN229", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN230: {value: OP_UNKNOWN230, name: "OP_UNKNOWN230", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN231: {value: OP_UNKNOWN231, name: "OP_UNKNOWN231", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN232: {value: OP_UNKNOWN232, name: "OP_UNKNOWN232", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN233: {value: OP_UNKNOWN233, name: "OP_UNKNOWN233", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN234: {value: OP_UNKNOWN234, name: "OP_UNKNOWN234", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN235: {value: OP_UNKNOWN235, name: "OP_UNKNOWN235", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN236: {value: OP_UNKNOWN236, name: "OP_UNKNOWN236", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN237: {value: OP_UNKNOWN237, name: "OP_UNKNOWN237", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN238: {value: OP_UNKNOWN238, name: "OP_UNKNOWN238", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN239: {value: OP_UNKNOWN239, name: "OP_UNKNOWN239", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN240: {value: OP_UNKNOWN240, name: "OP_UNKNOWN240", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN241: {value: OP_UNKNOWN241, name: "OP_UNKNOWN241", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN242: {value: OP_UNKNOWN242, name: "OP_UNKNOWN242", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN243: {value: OP_UNKNOWN243, name: "OP_UNKNOWN243", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN244: {value: OP_UNKNOWN244, name: "OP_UNKNOWN244", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN245: {value: OP_UNKNOWN245, name: "OP_UNKNOWN245", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN246: {value: OP_UNKNOWN246, name: "OP_UNKNOWN246", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN247: {value: OP_UNKNOWN247, name: "OP_UNKNOWN247", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN248: {value: OP_UNKNOWN248, name: "OP_UNKNOWN248", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN249: {value: OP_UNKNOWN249, name: "OP_UNKNOWN249", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN250: {value: OP_UNKNOWN250, name: "OP_UNKNOWN250", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN251: {value: OP_UNKNOWN251, name: "OP_UNKNOWN251", length: 1,
		opfunc: opcodeInvalid},
	OP_UNKNOWN252: {value: OP_UNKNOWN252, name: "OP_UNKNOWN252", length: 1,
		opfunc: opcodeInvalid},
	OP_PUBKEYHASH: {value: OP_PUBKEYHASH, name: "OP_PUBKEYHASH", length: 1,
		opfunc: opcodeInvalid},
	OP_PUBKEY: {value: OP_PUBKEY, name: "OP_PUBKEY", length: 1,
		opfunc: opcodeInvalid},
	OP_INVALIDOPCODE: {value: OP_INVALIDOPCODE, name: "OP_INVALIDOPCODE", length: 1,
		opfunc: opcodeInvalid},
}

// opcodeOnelineRepls defines opcode names which are replaced when doing a
// one-line disassembly.  This is done to match the output of the reference
// implementation while not changing the opcode names in the nicer full
// disassembly.
var opcodeOnelineRepls = map[string]string{
	"OP_1NEGATE": "-1",
	"OP_0":       "0",
	"OP_1":       "1",
	"OP_2":       "2",
	"OP_3":       "3",
	"OP_4":       "4",
	"OP_5":       "5",
	"OP_6":       "6",
	"OP_7":       "7",
	"OP_8":       "8",
	"OP_9":       "9",
	"OP_10":      "10",
	"OP_11":      "11",
	"OP_12":      "12",
	"OP_13":      "13",
	"OP_14":      "14",
	"OP_15":      "15",
	"OP_16":      "16",
}

type parsedOpcode struct {
	opcode *opcode
	data   []byte
	opfunc func(op parsedOpcode, s Script) error
}

// The following opcodes are disabled and are thus always bad to see in the
// instruction stream (even if turned off by a conditional).
func (pop *parsedOpcode) disabled() bool {
	switch pop.opcode.value {
	case OP_CAT:
		return true
	case OP_SUBSTR:
		return true
	case OP_LEFT:
		return true
	case OP_RIGHT:
		return true
	case OP_INVERT:
		return true
	case OP_AND:
		return true
	case OP_OR:
		return true
	case OP_XOR:
		return true
	case OP_2MUL:
		return true
	case OP_2DIV:
		return true
	case OP_MUL:
		return true
	case OP_DIV:
		return true
	case OP_MOD:
		return true
	case OP_LSHIFT:
		return true
	case OP_RSHIFT:
		return true
	default:
		return false
	}
}

// The following opcodes are always illegal when passed over by the program
// counter even if in a non-executed branch. (it isn't a coincidence that they
// are conditionals).
func (pop *parsedOpcode) alwaysIllegal() bool {
	switch pop.opcode.value {
	case OP_VERIF:
		return true
	case OP_VERNOTIF:
		return true
	default:
		return false
	}
}

// The following opcode are conditional and thus change the conditional
// execution stack state when passed.
func (pop *parsedOpcode) conditional() bool {
	switch pop.opcode.value {
	case OP_IF:
		return true
	case OP_NOTIF:
		return true
	case OP_ELSE:
		return true
	case OP_ENDIF:
		return true
	default:
		return false
	}
}

// exec peforms execution on the opcode. It takes into account whether or not
// it is hidden by conditionals, but some rules still must be tested in this
// case.
func (pop *parsedOpcode) exec(s *Script) error {
	// Disabled opcodes are ``fail on program counter''.
	if pop.disabled() {
		return StackErrOpDisabled
	}

	// Always-illegal opcodes are ``fail on program counter''.
	if pop.alwaysIllegal() {
		return StackErrReservedOpcode
	}

	// Note that this includes OP_RESERVED which counts as a push operation.
	if pop.opcode.value > OP_16 {
		s.numOps++
		if s.numOps > MaxOpsPerScript {
			return StackErrTooManyOperations
		}

	} else if len(pop.data) > MaxScriptElementSize {
		return StackErrElementTooBig
	}

	// If we are not a conditional opcode and we aren't executing, then
	// we are done now.
	if s.condStack[0] != OpCondTrue && !pop.conditional() {
		return nil
	}
	return pop.opcode.opfunc(pop, s)
}

func (pop *parsedOpcode) print(oneline bool) string {
	// The reference implementation one-line disassembly replaces opcodes
	// which represent values (e.g. OP_0 through OP_16 and OP_1NEGATE)
	// with the raw value.  However, when not doing a one-line dissassembly,
	// we prefer to show the actual opcode names.  Thus, only replace the
	// opcodes in question when the oneline flag is set.
	opcodeName := pop.opcode.name
	if oneline {
		if replName, ok := opcodeOnelineRepls[opcodeName]; ok {
			opcodeName = replName
		}
	}

	retString := opcodeName
	if pop.opcode.length == 1 {
		return retString
	}
	if oneline {
		retString = ""
	}
	if !oneline && pop.opcode.length < 0 {
		//add length to the end of retString
		retString += fmt.Sprintf(" 0x%0*x", 2*-pop.opcode.length,
			len(pop.data))
	}
	for _, val := range pop.data {
		if !oneline {
			retString += " "
		}
		retString += fmt.Sprintf("%02x", val)
	}
	return retString
}

func (pop *parsedOpcode) bytes() ([]byte, error) {
	var retbytes []byte
	if pop.opcode.length > 0 {
		retbytes = make([]byte, 1, pop.opcode.length)
	} else {
		retbytes = make([]byte, 1, 1+len(pop.data)-
			pop.opcode.length)
	}

	retbytes[0] = pop.opcode.value
	if pop.opcode.length == 1 {
		if len(pop.data) != 0 {
			return nil, StackErrInvalidOpcode
		}
		return retbytes, nil
	}
	nbytes := pop.opcode.length
	if pop.opcode.length < 0 {
		l := len(pop.data)
		// tempting just to hardcode to avoid the complexity here.
		switch pop.opcode.length {
		case -1:
			retbytes = append(retbytes, byte(l))
			nbytes = int(retbytes[1]) + len(retbytes)
		case -2:
			retbytes = append(retbytes, byte(l&0xff),
				byte(l>>8&0xff))
			nbytes = int(binary.LittleEndian.Uint16(retbytes[1:])) +
				len(retbytes)
		case -4:
			retbytes = append(retbytes, byte(l&0xff),
				byte((l>>8)&0xff), byte((l>>16)&0xff),
				byte((l>>24)&0xff))
			nbytes = int(binary.LittleEndian.Uint32(retbytes[1:])) +
				len(retbytes)
		}
	}

	retbytes = append(retbytes, pop.data...)

	if len(retbytes) != nbytes {
		return nil, StackErrInvalidOpcode
	}

	return retbytes, nil
}

// opcode implementation functions from here

func opcodeDisabled(op *parsedOpcode, s *Script) error {
	return StackErrOpDisabled
}

func opcodeReserved(op *parsedOpcode, s *Script) error {
	return StackErrReservedOpcode
}

// Recognised opcode, but for bitcoind internal use only.
func opcodeInvalid(op *parsedOpcode, s *Script) error {
	return StackErrInvalidOpcode
}

func opcodeFalse(op *parsedOpcode, s *Script) error {
	s.dstack.PushByteArray([]byte(""))

	return nil
}

func opcodePushData(op *parsedOpcode, s *Script) error {
	s.dstack.PushByteArray(op.data)
	return nil
}

func opcode1Negate(op *parsedOpcode, s *Script) error {
	s.dstack.PushInt(big.NewInt(-1))
	return nil
}

func opcodeN(op *parsedOpcode, s *Script) error {
	// 16 consecutive opcodes add increasing numbers to the stack.
	s.dstack.PushInt(big.NewInt(int64(op.opcode.value - (OP_1 - 1))))
	return nil
}

func opcodeNop(op *parsedOpcode, s *Script) error {
	// This page left intentionally blank
	return nil
}

// opcodeIf computes true/false based on the value on the stack and pushes
// the condition on the condStack (conditional execution stack)
func opcodeIf(op *parsedOpcode, s *Script) error {
	// opcodeIf will be executed even if it is on the non-execute side
	// of the conditional, this is so proper nesting is maintained
	var condval int
	if s.condStack[0] == OpCondTrue {
		ok, err := s.dstack.PopBool()
		if err != nil {
			return err
		}
		if ok {
			condval = OpCondTrue
		}
	} else {
		condval = OpCondSkip
	}
	cond := []int{condval}
	// push condition to the 'head' of the slice
	s.condStack = append(cond, s.condStack...)
	// TODO(drahn) check if a maximum condtitional stack limit exists
	return nil
}

// opcodeNotIf computes true/false based on the value on the stack and pushes
// the (inverted) condition on the condStack (conditional execution stack)
func opcodeNotIf(op *parsedOpcode, s *Script) error {
	// opcodeIf will be executed even if it is on the non-execute side
	// of the conditional, this is so proper nesting is maintained
	var condval int
	if s.condStack[0] == OpCondTrue {
		ok, err := s.dstack.PopBool()
		if err != nil {
			return err
		}
		if !ok {
			condval = OpCondTrue
		}
	} else {
		condval = OpCondSkip
	}
	cond := []int{condval}
	// push condition to the 'head' of the slice
	s.condStack = append(cond, s.condStack...)
	// TODO(drahn) check if a maximum condtitional stack limit exists
	return nil
}

// opcodeElse inverts conditional execution for other half of if/else/endif
func opcodeElse(op *parsedOpcode, s *Script) error {
	if len(s.condStack) < 2 {
		// intial true cannot be toggled, only pushed conditionals
		return StackErrNoIf
	}

	switch s.condStack[0] {
	case OpCondTrue:
		s.condStack[0] = OpCondFalse
	case OpCondFalse:
		s.condStack[0] = OpCondTrue
	case OpCondSkip:
		// value doesn't change in skip
	}
	return nil
}

// opcodeEndif terminates a conditional block, removing the  value from the
// conditional execution stack.
func opcodeEndif(op *parsedOpcode, s *Script) error {
	if len(s.condStack) < 2 {
		// intial true cannot be popped, only pushed conditionals
		return StackErrNoIf
	}

	stk := make([]int, len(s.condStack)-1, len(s.condStack)-1)
	copy(stk, s.condStack[1:])
	s.condStack = stk
	return nil
}

func opcodeVerify(op *parsedOpcode, s *Script) error {
	verified, err := s.dstack.PopBool()
	if err != nil {
		return err
	}

	if verified != true {
		return StackErrVerifyFailed
	}
	return nil
}

func opcodeReturn(op *parsedOpcode, s *Script) error {
	return StackErrEarlyReturn
}

func opcodeToAltStack(op *parsedOpcode, s *Script) error {
	so, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}
	s.astack.PushByteArray(so)

	return nil
}

func opcodeFromAltStack(op *parsedOpcode, s *Script) error {
	so, err := s.astack.PopByteArray()
	if err != nil {
		return err
	}
	s.dstack.PushByteArray(so)

	return nil
}

func opcode2Drop(op *parsedOpcode, s *Script) error {
	return s.dstack.DropN(2)
}

func opcode2Dup(op *parsedOpcode, s *Script) error {
	return s.dstack.DupN(2)
}

func opcode3Dup(op *parsedOpcode, s *Script) error {
	return s.dstack.DupN(3)
}

func opcode2Over(op *parsedOpcode, s *Script) error {
	return s.dstack.OverN(2)
}

func opcode2Rot(op *parsedOpcode, s *Script) error {
	return s.dstack.RotN(2)
}

func opcode2Swap(op *parsedOpcode, s *Script) error {
	return s.dstack.SwapN(2)
}

func opcodeIfDup(op *parsedOpcode, s *Script) error {
	val, err := s.dstack.PeekInt(0)
	if err != nil {
		return err
	}

	// Push copy of data iff it isn't zero
	if val.Sign() != 0 {
		s.dstack.PushInt(val)
	}

	return nil
}

func opcodeDepth(op *parsedOpcode, s *Script) error {
	s.dstack.PushInt(big.NewInt(int64(s.dstack.Depth())))
	return nil
}

func opcodeDrop(op *parsedOpcode, s *Script) error {
	return s.dstack.DropN(1)
}

func opcodeDup(op *parsedOpcode, s *Script) error {
	return s.dstack.DupN(1)
}

func opcodeNip(op *parsedOpcode, s *Script) error {
	return s.dstack.NipN(1)
}

func opcodeOver(op *parsedOpcode, s *Script) error {
	return s.dstack.OverN(1)
}

// Copy object N items back in the stack to the top. Where N is the value in
// the top of the stack.
func opcodePick(op *parsedOpcode, s *Script) error {
	pidx, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	// PopInt promises that the int returned is 32 bit.
	val := int(pidx.Int64())

	return s.dstack.PickN(val)
}

// Move object N items back in the stack to the top. Where N is the value in
// the top of the stack.
func opcodeRoll(op *parsedOpcode, s *Script) error {
	ridx, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	// PopInt promises that the int returned is 32 bit.
	val := int(ridx.Int64())

	return s.dstack.RollN(val)
}

// Rotate top three items on the stack to the left.
// e.g. 1,2,3 -> 2,3,1
func opcodeRot(op *parsedOpcode, s *Script) error {
	return s.dstack.RotN(1)
}

// Swap the top two items on the stack: 1,2 -> 2,1
func opcodeSwap(op *parsedOpcode, s *Script) error {
	return s.dstack.SwapN(1)
}

// The item at the top of the stack is copied and inserted before the
// second-to-top item. e.g.: 2,1, -> 2,1,2
func opcodeTuck(op *parsedOpcode, s *Script) error {
	return s.dstack.Tuck()
}

// Push the size of the item on top of the stack onto the stack.
func opcodeSize(op *parsedOpcode, s *Script) error {
	i, err := s.dstack.PeekByteArray(0)
	if err != nil {
		return err
	}

	s.dstack.PushInt(big.NewInt(int64(len(i))))
	return nil
}

func opcodeEqual(op *parsedOpcode, s *Script) error {
	a, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}
	b, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushBool(bytes.Equal(a, b))
	return nil
}

func opcodeEqualVerify(op *parsedOpcode, s *Script) error {
	err := opcodeEqual(op, s)
	if err == nil {
		err = opcodeVerify(op, s)
	}
	return err
}

func opcode1Add(op *parsedOpcode, s *Script) error {
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	s.dstack.PushInt(new(big.Int).Add(m, big.NewInt(1)))

	return nil
}

func opcode1Sub(op *parsedOpcode, s *Script) error {
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}
	s.dstack.PushInt(new(big.Int).Sub(m, big.NewInt(1)))

	return nil
}

func opcodeNegate(op *parsedOpcode, s *Script) error {
	// XXX when we remove types just flip the 0x80 bit of msb
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	s.dstack.PushInt(new(big.Int).Neg(m))
	return nil
}

func opcodeAbs(op *parsedOpcode, s *Script) error {
	// XXX when we remove types just &= ~0x80 on msb
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	s.dstack.PushInt(new(big.Int).Abs(m))

	return nil
}

// If then input is 0 or 1, it is flipped. Otherwise the output will be 0.
// (n.b. official client just has 1 is 0, else 0)
func opcodeNot(op *parsedOpcode, s *Script) error {
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}
	if m.Sign() == 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}
	return nil
}

// opcode returns 0 if the input is 0, 1 otherwise.
func opcode0NotEqual(op *parsedOpcode, s *Script) error {
	m, err := s.dstack.PopInt()
	if err != nil {
		return err
	}
	if m.Sign() != 0 {
		m.SetInt64(1)
	}
	s.dstack.PushInt(m)

	return nil
}

// Push result of adding top two entries on stack
func opcodeAdd(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	s.dstack.PushInt(new(big.Int).Add(v0, v1))
	return nil
}

// Push result of subtracting 2nd entry on stack from first.
func opcodeSub(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	s.dstack.PushInt(new(big.Int).Sub(v1, v0))
	return nil
}

// If both of the top two entries on the stack are not zero output is 1.
// Otherwise, 0.
func opcodeBoolAnd(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v0.Sign() != 0 && v1.Sign() != 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

// If either of the top two entries on the stack are not zero output is 1.
// Otherwise, 0.
func opcodeBoolOr(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v0.Sign() != 0 || v1.Sign() != 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

func opcodeNumEqual(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v0.Cmp(v1) == 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

func opcodeNumEqualVerify(op *parsedOpcode, s *Script) error {
	err := opcodeNumEqual(op, s)
	if err == nil {
		err = opcodeVerify(op, s)
	}
	return err
}

func opcodeNumNotEqual(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v0.Cmp(v1) != 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

func opcodeLessThan(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) == -1 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

func opcodeGreaterThan(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) == 1 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}
	return nil
}

func opcodeLessThanOrEqual(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) <= 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}
	return nil
}

func opcodeGreaterThanOrEqual(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) >= 0 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}

	return nil
}

func opcodeMin(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) == -1 {
		s.dstack.PushInt(new(big.Int).Set(v1))
	} else {
		s.dstack.PushInt(new(big.Int).Set(v0))
	}

	return nil
}

func opcodeMax(op *parsedOpcode, s *Script) error {
	v0, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	v1, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if v1.Cmp(v0) == 1 {
		s.dstack.PushInt(new(big.Int).Set(v1))
	} else {
		s.dstack.PushInt(new(big.Int).Set(v0))
	}

	return nil
}

// stack input: x, min, max. Returns 1 if x is within specified range
// (left inclusive), 0 otherwise
func opcodeWithin(op *parsedOpcode, s *Script) error {
	maxVal, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	minVal, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	x, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	if x.Cmp(minVal) >= 0 && x.Cmp(maxVal) == -1 {
		s.dstack.PushInt(big.NewInt(1))
	} else {
		s.dstack.PushInt(big.NewInt(0))
	}
	return nil
}

// Calculate the hash of hasher over buf.
func calcHash(buf []byte, hasher hash.Hash) []byte {
	hasher.Write(buf)
	return hasher.Sum(nil)
}

// calculate hash160 which is ripemd160(sha256(data))
func calcHash160(buf []byte) []byte {
	return calcHash(calcHash(buf, fastsha256.New()), ripemd160.New())
}

func opcodeRipemd160(op *parsedOpcode, s *Script) error {
	buf, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushByteArray(calcHash(buf, ripemd160.New()))
	return nil
}

func opcodeSha1(op *parsedOpcode, s *Script) error {
	buf, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushByteArray(calcHash(buf, sha1.New()))
	return nil
}

func opcodeSha256(op *parsedOpcode, s *Script) error {
	buf, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushByteArray(calcHash(buf, fastsha256.New()))
	return nil
}

func opcodeHash160(op *parsedOpcode, s *Script) error {
	buf, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushByteArray(calcHash160(buf))
	return nil
}

func opcodeHash256(op *parsedOpcode, s *Script) error {
	buf, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	s.dstack.PushByteArray(btcwire.DoubleSha256(buf))
	return nil
}

func opcodeCodeSeparator(op *parsedOpcode, s *Script) error {
	s.lastcodesep = s.scriptoff

	return nil
}

func opcodeCheckSig(op *parsedOpcode, s *Script) error {

	pkStr, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	sigStr, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	// Signature actually needs needs to be longer than this, but we need
	// at least  1 byte for the below. btcec will check full length upon
	// parsing the signature.
	if len(sigStr) < 1 {
		s.dstack.PushBool(false)
		return nil
	}

	// Trim off hashtype from the signature string.
	hashType := sigStr[len(sigStr)-1]
	sigStr = sigStr[:len(sigStr)-1]

	// Get script from the last OP_CODESEPARATOR and without any subsequent
	// OP_CODESEPARATORs
	subScript := s.subScript()

	// Unlikely to hit any cases here, but remove the signature from
	// the script if present.
	subScript = removeOpcodeByData(subScript, sigStr)

	hash := calcScriptHash(subScript, hashType, &s.tx, s.txidx)

	pubKey, err := btcec.ParsePubKey(pkStr, btcec.S256())
	if err != nil {
		s.dstack.PushBool(false)
		return nil
	}

	var signature *btcec.Signature
	if s.der {
		signature, err = btcec.ParseDERSignature(sigStr, btcec.S256())
	} else {
		signature, err = btcec.ParseSignature(sigStr, btcec.S256())
	}
	if err != nil {
		s.dstack.PushBool(false)
		return nil
	}

	log.Tracef("%v", newLogClosure(func() string {
		return fmt.Sprintf("op_checksig pubKey %v\npk.x: %v\n "+
			"pk.y: %v\n r: %v\n s: %v\ncheckScriptHash  %v",
			spew.Sdump(pkStr), pubKey.X, pubKey.Y,
			signature.R, signature.S, spew.Sdump(hash))
	}))
	ok := ecdsa.Verify(pubKey.ToECDSA(), hash, signature.R, signature.S)
	s.dstack.PushBool(ok)
	return nil
}

func opcodeCheckSigVerify(op *parsedOpcode, s *Script) error {
	err := opcodeCheckSig(op, s)
	if err == nil {
		err = opcodeVerify(op, s)
	}
	return err
}

type sig struct {
	s  *btcec.Signature
	ht byte
}

// stack; sigs <numsigs> pubkeys <numpubkeys>
func opcodeCheckMultiSig(op *parsedOpcode, s *Script) error {

	numPubkeys, err := s.dstack.PopInt()
	if err != nil {
		return err
	}

	// XXX arbitrary limits
	// nore more than 20 pubkeyhs, or 201 operations

	// PopInt promises that the int returned is 32 bit.
	npk := int(numPubkeys.Int64())
	if npk < 0 || npk > MaxPubKeysPerMultiSig {
		return StackErrTooManyPubkeys
	}
	s.numOps += npk
	if s.numOps > MaxOpsPerScript {
		return StackErrTooManyOperations
	}
	pubKeyStrings := make([][]byte, npk)
	pubKeys := make([]*btcec.PublicKey, npk)
	for i := range pubKeys {
		pubKeyStrings[i], err = s.dstack.PopByteArray()
		if err != nil {
			return err
		}
	}

	numSignatures, err := s.dstack.PopInt()
	if err != nil {
		return err
	}
	// PopInt promises that the int returned is 32 bit.
	nsig := int(numSignatures.Int64())
	if nsig < 0 {
		return fmt.Errorf("number of signatures %d is less than 0", nsig)
	}
	if nsig > npk {
		return fmt.Errorf("more signatures than pubkeys: %d > %d", nsig, npk)
	}

	sigStrings := make([][]byte, nsig)
	signatures := make([]sig, 0, nsig)
	for i := range sigStrings {
		sigStrings[i], err = s.dstack.PopByteArray()
		if err != nil {
			return err
		}
		if len(sigStrings[i]) == 0 {
			continue
		}
		sig := sig{}
		sig.ht = sigStrings[i][len(sigStrings[i])-1]
		// skip off the last byte for hashtype
		if s.der {
			sig.s, err =
				btcec.ParseDERSignature(
					sigStrings[i][:len(sigStrings[i])-1],
					btcec.S256())
		} else {
			sig.s, err =
				btcec.ParseSignature(
					sigStrings[i][:len(sigStrings[i])-1],
					btcec.S256())
		}
		if err == nil {
			signatures = append(signatures, sig)
		}
	}

	// bug in bitcoind mean we pop one more stack value than should be used.
	dummy, err := s.dstack.PopByteArray()
	if err != nil {
		return err
	}

	if s.strictMultiSig && len(dummy) != 0 {
		return fmt.Errorf("multisig dummy argument is not zero length: %d",
			len(dummy))
	}

	if len(signatures) == 0 {
		s.dstack.PushBool(nsig == 0)
		return nil
	}

	// Trim OP_CODESEPARATORs
	script := s.subScript()

	// Remove any of the signatures that happen to be in the script.
	// can't sign somthing containing the signature you're making, after
	// all
	for i := range sigStrings {
		script = removeOpcodeByData(script, sigStrings[i])
	}

	curPk := 0
	for i := range signatures {
		// check signatures.
		success := false

		hash := calcScriptHash(script, signatures[i].ht, &s.tx, s.txidx)
	inner:
		// Find first pubkey that successfully validates signature.
		// we start off the search from the key that was successful
		// last time.
		for ; curPk < len(pubKeys); curPk++ {
			if pubKeys[curPk] == nil {
				pubKeys[curPk], err =
					btcec.ParsePubKey(pubKeyStrings[curPk],
						btcec.S256())
				if err != nil {
					continue
				}
			}
			success = ecdsa.Verify(pubKeys[curPk].ToECDSA(), hash,
				signatures[i].s.R, signatures[i].s.S)
			if success {
				break inner
			}
		}
		if success == false {
			s.dstack.PushBool(false)
			return nil
		}
	}
	s.dstack.PushBool(true)

	return nil
}

func opcodeCheckMultiSigVerify(op *parsedOpcode, s *Script) error {
	err := opcodeCheckMultiSig(op, s)
	if err == nil {
		err = opcodeVerify(op, s)
	}
	return err
}
