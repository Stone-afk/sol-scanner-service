package parser

import (
	"encoding/binary"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
)

// 指令数据里的定长字段宽度。
const (
	u8Len        = 1
	u32Len       = 4
	u64Len       = 8
	pubkeyLen    = 32
	splFeeHeader = u8Len + u8Len
)

// System 指令判别符，小端 u32。
const (
	sysCreateAccount         uint32 = 0
	sysTransfer              uint32 = 2
	sysCreateAccountWithSeed uint32 = 3
	sysWithdrawNonce         uint32 = 5
	sysTransferWithSeed      uint32 = 11
)

// SPL Token / Token-2022 指令判别符，u8。
const (
	splTransfer           byte = 3
	splMintTo             byte = 7
	splBurn               byte = 8
	splTransferChecked    byte = 12
	splMintToChecked      byte = 14
	splBurnChecked        byte = 15
	splTransferFeeExt     byte = 26
	splTransferCheckedFee byte = 1
)

type decodedTransfer struct {
	program     domain.Program
	kind        domain.TransferKind
	from, to    string
	mint        string
	decimals    uint8
	hasDecimals bool
	amount      uint64
	fee         uint64
	hasFee      bool
}

// decodeInstruction decodes one in-scope transfer instruction.
// ok=false means out of scope; err means in-scope but malformed.
func decodeInstruction(programID solana.PublicKey, accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	switch {
	case programID.Equals(solana.SystemProgramID):
		return decodeSystem(accounts, data)
	case programID.Equals(solana.TokenProgramID):
		return decodeSPLToken(domain.ProgramSPLToken, accounts, data)
	case programID.Equals(solana.Token2022ProgramID):
		return decodeToken2022(accounts, data)
	default:
		return decodedTransfer{}, false, nil
	}
}

func decodeSystem(accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	if len(data) < u32Len {
		// Not enough to read a System discriminator — treat as truncated in-scope attempt
		// only when we cannot classify; short buffer with no disc is DecodeError for System
		// callers that already selected System program (always in-family).
		return decodedTransfer{}, false, DecodeError{Reason: "data"}
	}
	disc := binary.LittleEndian.Uint32(data[:u32Len])
	switch disc {
	case sysTransfer:
		if len(data) < u32Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, to, err := requireAccounts(accounts, 0, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: domain.ProgramSystem,
			kind:    domain.KindTransfer,
			from:    from,
			to:      to,
			amount:  binary.LittleEndian.Uint64(data[u32Len : u32Len+u64Len]),
		}, true, nil
	case sysTransferWithSeed:
		if len(data) < u32Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		to, err := requireAccount(accounts, 2)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: domain.ProgramSystem,
			kind:    domain.KindTransferWithSeed,
			from:    from,
			to:      to,
			amount:  binary.LittleEndian.Uint64(data[u32Len : u32Len+u64Len]),
		}, true, nil
	case sysCreateAccount:
		if len(data) < u32Len+u64Len+u64Len+pubkeyLen {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, to, err := requireAccounts(accounts, 0, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: domain.ProgramSystem,
			kind:    domain.KindCreateAccount,
			from:    from,
			to:      to,
			amount:  binary.LittleEndian.Uint64(data[u32Len : u32Len+u64Len]),
		}, true, nil
	case sysCreateAccountWithSeed:
		from, to, err := requireAccounts(accounts, 0, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		lamports, err := readCreateAccountWithSeedLamports(data)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: domain.ProgramSystem,
			kind:    domain.KindCreateAccountWithSeed,
			from:    from,
			to:      to,
			amount:  lamports,
		}, true, nil
	case sysWithdrawNonce:
		if len(data) < u32Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, to, err := requireAccounts(accounts, 0, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: domain.ProgramSystem,
			kind:    domain.KindWithdrawNonceAccount,
			from:    from,
			to:      to,
			amount:  binary.LittleEndian.Uint64(data[u32Len : u32Len+u64Len]),
		}, true, nil
	default:
		return decodedTransfer{}, false, nil
	}
}

func readCreateAccountWithSeedLamports(data []byte) (uint64, error) {
	// disc + base pubkey + seed_len, then seed, then lamports.
	const minBeforeSeed = u32Len + pubkeyLen + u64Len
	if len(data) < minBeforeSeed {
		return 0, DecodeError{Reason: "data"}
	}
	seedLen := binary.LittleEndian.Uint64(data[u32Len+pubkeyLen : u32Len+pubkeyLen+u64Len])
	off := minBeforeSeed + int(seedLen)
	if seedLen > uint64(len(data)) || off+u64Len > len(data) {
		return 0, DecodeError{Reason: "data"}
	}
	return binary.LittleEndian.Uint64(data[off : off+u64Len]), nil
}

func decodeToken2022(accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	if len(data) >= u8Len && data[0] == splTransferFeeExt {
		if len(data) < splFeeHeader {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		if data[u8Len] != splTransferCheckedFee {
			return decodedTransfer{}, false, nil
		}
		// transferCheckedWithFee: u8,u8,u64 amount,u8 decimals,u64 fee
		if len(data) < splFeeHeader+u64Len+u8Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		mint, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		to, err := requireAccount(accounts, 2)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program:     domain.ProgramToken2022,
			kind:        domain.KindTransferCheckedWithFee,
			from:        from,
			to:          to,
			mint:        mint,
			amount:      binary.LittleEndian.Uint64(data[splFeeHeader : splFeeHeader+u64Len]),
			decimals:    data[splFeeHeader+u64Len],
			hasDecimals: true,
			fee:         binary.LittleEndian.Uint64(data[splFeeHeader+u64Len+u8Len : splFeeHeader+u64Len+u8Len+u64Len]),
			hasFee:      true,
		}, true, nil
	}
	return decodeSPLToken(domain.ProgramToken2022, accounts, data)
}

func decodeSPLToken(program domain.Program, accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	if len(data) < u8Len {
		return decodedTransfer{}, false, DecodeError{Reason: "data"}
	}
	switch data[0] {
	case splTransfer: // mint backfilled later
		if len(data) < u8Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, to, err := requireAccounts(accounts, 0, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: program,
			kind:    domain.KindTransfer,
			from:    from,
			to:      to,
			amount:  binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
		}, true, nil
	case splTransferChecked:
		if len(data) < u8Len+u64Len+u8Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		mint, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		to, err := requireAccount(accounts, 2)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program:     program,
			kind:        domain.KindTransferChecked,
			from:        from,
			to:          to,
			mint:        mint,
			amount:      binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
			decimals:    data[u8Len+u64Len],
			hasDecimals: true,
		}, true, nil
	case splMintTo:
		if len(data) < u8Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		mint, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		to, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: program,
			kind:    domain.KindMintTo,
			to:      to,
			mint:    mint,
			amount:  binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
		}, true, nil
	case splMintToChecked:
		if len(data) < u8Len+u64Len+u8Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		mint, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		to, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program:     program,
			kind:        domain.KindMintToChecked,
			to:          to,
			mint:        mint,
			amount:      binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
			decimals:    data[u8Len+u64Len],
			hasDecimals: true,
		}, true, nil
	case splBurn:
		if len(data) < u8Len+u64Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		mint, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program: program,
			kind:    domain.KindBurn,
			from:    from,
			mint:    mint,
			amount:  binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
		}, true, nil
	case splBurnChecked:
		if len(data) < u8Len+u64Len+u8Len {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		from, err := requireAccount(accounts, 0)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		mint, err := requireAccount(accounts, 1)
		if err != nil {
			return decodedTransfer{}, true, err
		}
		return decodedTransfer{
			program:     program,
			kind:        domain.KindBurnChecked,
			from:        from,
			mint:        mint,
			amount:      binary.LittleEndian.Uint64(data[u8Len : u8Len+u64Len]),
			decimals:    data[u8Len+u64Len],
			hasDecimals: true,
		}, true, nil
	default:
		return decodedTransfer{}, false, nil
	}
}

func requireAccounts(accounts []solana.PublicKey, i, j int) (string, string, error) {
	a, err := requireAccount(accounts, i)
	if err != nil {
		return "", "", err
	}
	b, err := requireAccount(accounts, j)
	if err != nil {
		return "", "", err
	}
	return a, b, nil
}

func requireAccount(accounts []solana.PublicKey, i int) (string, error) {
	if i >= len(accounts) {
		return "", DecodeError{Reason: "accounts"}
	}
	return accounts[i].String(), nil
}
