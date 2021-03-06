package parser

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
)

// SlpGenesis is an unmarshalled Genesis OP_RETURN
type SlpGenesis struct {
	Ticker, Name, DocumentURI, DocumentHash []byte
	Decimals, MintBatonVout                 int
	Qty                                     uint64
}

// TickerAsUtf8 converts ticker field bytes to string using utf8 decoding
func (g *SlpGenesis) TickerAsUtf8() string {
	return string(g.Ticker)
}

// NameAsUtf8 converts name field bytes to string using utf8 decoding
func (g *SlpGenesis) NameAsUtf8() string {
	return string(g.Name)
}

// DocumentURIAsUtf8 converts documentURI field bytes to string using utf8 decoding
func (g *SlpGenesis) DocumentURIAsUtf8() string {
	return string(g.DocumentURI)
}

// DocumentHashAsHex converts documentHash field bytes to string using hexidecimal encoding
func (g *SlpGenesis) DocumentHashAsHex() string {
	return hex.EncodeToString(g.DocumentHash)
}

// SlpMint is an unmarshalled Mint OP_RETURN
type SlpMint struct {
	TokenID       []byte
	MintBatonVout int
	Qty           uint64
}

// TokenIDAsHex converts TokenId field bytes to string using hexidecimal encoding
func (m *SlpMint) TokenIDAsHex() string {
	return hex.EncodeToString(m.TokenID)
}

// SlpSend is an unmarshalled Send OP_RETURN
type SlpSend struct {
	TokenID []byte
	Amounts []uint64
}

// TokenIDAsHex converts TokenId field bytes to string using hexidecimal encoding
func (s *SlpSend) TokenIDAsHex() string {
	return hex.EncodeToString(s.TokenID)
}

// SlpOpReturn represents a generic interface for
// any type of unmarshalled SLP OP_RETURN message
type SlpOpReturn interface {
	// TODO: once tests are added may need to add ToMap to simplify
	//		 interaction with the SLP unit tests
	//ToMap(raw bool) map[string]string
}

// ParseResult returns the parsed result.
type ParseResult struct {
	TokenType       int
	TransactionType string
	Data            SlpOpReturn
}

// ParseSLP unmarshalls an SLP message from a transaction scriptPubKey.
func ParseSLP(scriptPubKey []byte) (*ParseResult, error) {
	it := 0
	itObj := scriptPubKey

	const OP_0 int = 0x00
	const OP_RETURN int = 0x6a
	const OP_PUSHDATA1 int = 0x4c
	const OP_PUSHDATA2 int = 0x4d
	const OP_PUSHDATA4 int = 0x4e

	extractU8 := func() int {
		r := uint8(itObj[it : it+1][0])
		it++
		return int(r)
	}

	extractU16 := func(littleEndian bool) int {
		var r uint16
		if littleEndian {
			r = binary.LittleEndian.Uint16(itObj[it : it+2])
		} else {
			r = binary.BigEndian.Uint16(itObj[it : it+2])
		}
		it += 2
		return int(r)
	}

	extractU32 := func(littleEndian bool) int {
		var r uint32
		if littleEndian {
			r = binary.LittleEndian.Uint32(itObj[it : it+4])
		} else {
			r = binary.BigEndian.Uint32(itObj[it : it+4])
		}
		it += 4
		return int(r)
	}

	extractU64 := func(littleEndian bool) int {
		var r uint64
		if littleEndian {
			r = binary.LittleEndian.Uint64(itObj[it : it+8])
		} else {
			r = binary.BigEndian.Uint64(itObj[it : it+8])
		}
		return int(r)
	}

	if err := parseCheck(len(itObj) == 0, "scriptpubkey cannot be empty"); err != nil {
		return nil, err
	}

	if err := parseCheck(int(itObj[it]) != OP_RETURN, "scriptpubkey not op_return"); err != nil {
		return nil, err
	}

	if err := parseCheck(len(itObj) < 10, "scriptpubkey too small"); err != nil {
		return nil, err
	}

	it++

	extractPushdata := func() int {
		if it == len(itObj) {
			return -1
		}
		cnt := extractU8()
		if cnt > OP_0 && cnt < OP_PUSHDATA1 {
			if it+cnt > len(itObj) {
				it--
				return -1
			}
			return cnt
		} else if cnt == OP_PUSHDATA1 {
			if it+1 >= len(itObj) {
				it--
				return -1
			}
			return extractU8()
		} else if cnt == OP_PUSHDATA2 {
			if it+2 >= len(itObj) {
				it--
				return -1
			}
			return extractU16(true)
		} else if cnt == OP_PUSHDATA4 {
			if it+4 >= len(itObj) {
				it--
				return -1
			}
			return extractU32(true)
		}
		// other opcodes not allowed
		it--
		return -1
	}

	bufferToBN := func() (int, error) {
		if len(itObj) == 1 {
			return extractU8(), nil
		}
		if len(itObj) == 2 {
			return extractU16(false), nil
		}
		if len(itObj) == 4 {
			return extractU32(false), nil
		}
		if len(itObj) == 8 {
			return extractU64(false), nil
		}
		return 0, errors.New("extraction of number from buffer failed")
	}

	checkValidTokenID := func(tokenID []byte) bool {
		return len(tokenID) == 32
	}

	chunks := make([][]byte, 0)
	for _len := extractPushdata(); _len >= 0; _len = extractPushdata() {
		buf := make([]byte, _len)
		copy(buf, itObj[it:it+_len])

		if err := parseCheck(it+_len > len(itObj), "pushdata data extraction failed"); err != nil {
			return nil, err
		}

		it += _len
		chunks = append(chunks, buf)
		if len(chunks) == 1 {
			lokadID := chunks[0]

			if err := parseCheck(len(lokadID) != 4, "lokad id wrong size"); err != nil {
				return nil, err
			}

			if err := parseCheck(
				string(lokadID[0]) != "S" ||
					string(lokadID[1]) != "L" ||
					string(lokadID[2]) != "P" ||
					lokadID[3] != 0x00, "SLP not in first chunk",
			); err != nil {
				return nil, err
			}

		}
	}

	if err := parseCheck(it != len(itObj), "trailing data"); err != nil {
		return nil, err
	}

	if err := parseCheck(len(chunks) == 0, "chunks empty"); err != nil {
		return nil, err
	}

	cit := 0

	checkNext := func() error {
		cit++

		if err := parseCheck(cit == len(chunks), "parsing ended early"); err != nil {
			return err
		}

		it = 0
		itObj = chunks[cit]

		return nil
	}

	if err := checkNext(); err != nil {
		return nil, err
	}

	tokenTypeBuf := itObj

	if err := parseCheck(len(tokenTypeBuf) != 1 && len(tokenTypeBuf) != 2,
		"token_type string length must be 1 or 2"); err != nil {
		return nil, err
	}

	tokenType, err := bufferToBN()
	if err != nil {
		return nil, err
	}

	if err := parseCheck(tokenType != 0x01 &&
		tokenType != 0x41 &&
		tokenType != 0x81,
		"token_type not token-type1, nft1-group, or nft1-child"); err != nil {
		return nil, err
	}

	if err := checkNext(); err != nil {
		return nil, err
	}

	transactionType := string(itObj)
	if transactionType == "GENESIS" {

		if err := parseCheck(len(chunks) != 10, "wrong number of chunks"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		ticker := itObj
		if err := checkNext(); err != nil {
			return nil, err
		}

		name := itObj
		if err := checkNext(); err != nil {
			return nil, err
		}

		documentURI := itObj
		if err := checkNext(); err != nil {
			return nil, err
		}

		documentHash := itObj

		if err := parseCheck(len(documentHash) != 0 && len(documentHash) != 32, "documentHash must be size 0 or 32"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		decimalsBuf := itObj

		if err := parseCheck(len(decimalsBuf) != 1, "decimals string length must be 1"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		decimals, err := bufferToBN()
		if err != nil {
			return nil, err
		}

		if err := parseCheck(decimals > 9, "decimals biger than 9"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		mintBatonVoutBuf := itObj
		mintBatonVout := 0

		if err := parseCheck(len(mintBatonVoutBuf) >= 2, "mintBatonVout string must be 0 or 1"); err != nil {
			return nil, err
		}

		if len(mintBatonVoutBuf) > 0 {
			mintBatonVout, err = bufferToBN()
			if err != nil {
				return nil, err
			}

			if err := parseCheck(mintBatonVout < 2, "mintBatonVout must be at least 2"); err != nil {
				return nil, err
			}
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		qtyBuf := itObj

		if err := parseCheck(len(qtyBuf) != 8, "initialQty Must be provided as an 8-byte buffer"); err != nil {
			return nil, err
		}

		qty, err := bufferToBN()
		if err != nil {
			return nil, err
		}

		if tokenType == 0x41 {
			if err := parseCheck(decimals != 0, "NFT1 child token must have divisibility set to 0 decimal places"); err != nil {
				return nil, err
			}

			if err := parseCheck(mintBatonVout != 0, "NFT1 child token must not have a minting baton"); err != nil {
				return nil, err
			}

			if err := parseCheck(qty != 1, "NFT1 child token must have quantity of 1"); err != nil {
				return nil, err
			}
		}

		return &ParseResult{
			TokenType:       tokenType,
			TransactionType: transactionType,
			Data: SlpGenesis{
				Ticker:        ticker,
				Name:          name,
				DocumentURI:   documentURI,
				DocumentHash:  documentHash,
				Decimals:      decimals,
				MintBatonVout: mintBatonVout,
				Qty:           uint64(qty),
			},
		}, nil
	} else if transactionType == "MINT" {

		if err := parseCheck(tokenType == 0x41, "NFT1 Child cannot have MINT transaction type."); err != nil {
			return nil, err
		}

		if err := parseCheck(len(chunks) != 6, "wrong number of chunks"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		tokenID := itObj

		if err := parseCheck(!checkValidTokenID(tokenID), "tokenID invalid size"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		mintBatonVoutBuf := itObj
		mintBatonVout := 0

		if err := parseCheck(len(mintBatonVoutBuf) >= 2, "mint_baton_vout string length must be 0 or 1"); err != nil {
			return nil, err
		}

		if len(mintBatonVoutBuf) > 0 {
			mintBatonVout, err = bufferToBN()
			if err != nil {
				return nil, err
			}

			if err := parseCheck(mintBatonVout < 2, "mint_baton_vout must be at least 2"); err != nil {
				return nil, err
			}

		}
		if err := checkNext(); err != nil {
			return nil, err
		}

		addiitionalQtyBuf := itObj

		if err := parseCheck(len(addiitionalQtyBuf) != 8, "additional_qty must be provided as an 8-byte buffer"); err != nil {
			return nil, err
		}

		qty, err := bufferToBN()
		if err != nil {
			return nil, err
		}

		return &ParseResult{
			TokenType:       tokenType,
			TransactionType: transactionType,
			Data: SlpMint{
				TokenID:       tokenID,
				MintBatonVout: mintBatonVout,
				Qty:           uint64(qty),
			},
		}, nil
	} else if transactionType == "SEND" {

		if err := parseCheck(len(chunks) < 4, "wrong number of chunks"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		tokenID := itObj

		if err := parseCheck(!checkValidTokenID(tokenID), "tokenId invalid size"); err != nil {
			return nil, err
		}

		if err := checkNext(); err != nil {
			return nil, err
		}

		amounts := make([]uint64, 0)
		for cit != len(chunks) {
			amountBuf := itObj

			if err := parseCheck(len(amountBuf) != 8, "amount string size not 8 bytes"); err != nil {
				return nil, err
			}

			value, err := bufferToBN()
			if err != nil {
				return nil, err
			}
			amounts = append(amounts, uint64(value))

			cit++
			if cit < len(chunks) {
				itObj = chunks[cit]
			}
			it = 0
		}

		if err := parseCheck(len(amounts) == 0, "token_amounts size is 0"); err != nil {
			return nil, err
		}

		if err := parseCheck(len(amounts) > 19, "token_amounts size is greater than 19"); err != nil {
			return nil, err
		}

		return &ParseResult{
			TokenType:       tokenType,
			TransactionType: transactionType,
			Data: SlpSend{
				TokenID: tokenID,
				Amounts: amounts,
			},
		}, nil
	}

	return nil, errors.New("impossible parsing result")
}

func parseCheck(v bool, str string) error {
	if v {
		return errors.New(str)
	}

	return nil
}
