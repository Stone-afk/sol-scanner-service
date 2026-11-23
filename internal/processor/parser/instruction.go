package parser

import (
	"encoding/binary"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
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
	if len(data) < 4 {
		// Not enough to read a System discriminator — treat as truncated in-scope attempt
		// only when we cannot classify; short buffer with no disc is DecodeError for System
		// callers that already selected System program (always in-family).
		return decodedTransfer{}, false, DecodeError{Reason: "data"}
	}
	disc := binary.LittleEndian.Uint32(data[:4])
	switch disc {
	case 2: // transfer
		if len(data) < 4+8 {
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
			amount:  binary.LittleEndian.Uint64(data[4:12]),
		}, true, nil
	case 11: // transferWithSeed
		if len(data) < 4+8 {
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
			amount:  binary.LittleEndian.Uint64(data[4:12]),
		}, true, nil
	case 0: // createAccount
		if len(data) < 4+8+8+32 {
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
			amount:  binary.LittleEndian.Uint64(data[4:12]),
		}, true, nil
	case 3: // createAccountWithSeed
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
	case 5: // withdrawNonceAccount
		if len(data) < 4+8 {
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
			amount:  binary.LittleEndian.Uint64(data[4:12]),
		}, true, nil
	default:
		return decodedTransfer{}, false, nil
	}
}

func readCreateAccountWithSeedLamports(data []byte) (uint64, error) {
	// disc(4) + base(32) + seed_len(8) + seed + lamports(8) + space(8) + owner(32)
	const minBeforeSeed = 4 + 32 + 8
	if len(data) < minBeforeSeed {
		return 0, DecodeError{Reason: "data"}
	}
	seedLen := binary.LittleEndian.Uint64(data[4+32 : 4+32+8])
	off := minBeforeSeed + int(seedLen)
	if seedLen > uint64(len(data)) || off+8 > len(data) {
		return 0, DecodeError{Reason: "data"}
	}
	return binary.LittleEndian.Uint64(data[off : off+8]), nil
}

func decodeToken2022(accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	if len(data) >= 1 && data[0] == 26 {
		if len(data) < 2 {
			return decodedTransfer{}, true, DecodeError{Reason: "data"}
		}
		if data[1] != 1 {
			return decodedTransfer{}, false, nil
		}
		// transferCheckedWithFee: u8,u8,u64 amount,u8 decimals,u64 fee
		if len(data) < 2+8+1+8 {
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
			amount:      binary.LittleEndian.Uint64(data[2:10]),
			decimals:    data[10],
			hasDecimals: true,
			fee:         binary.LittleEndian.Uint64(data[11:19]),
			hasFee:      true,
		}, true, nil
	}
	return decodeSPLToken(domain.ProgramToken2022, accounts, data)
}

func decodeSPLToken(program domain.Program, accounts []solana.PublicKey, data []byte) (decodedTransfer, bool, error) {
	if len(data) < 1 {
		return decodedTransfer{}, false, DecodeError{Reason: "data"}
	}
	switch data[0] {
	case 3: // transfer — mint backfilled later
		if len(data) < 1+8 {
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
			amount:  binary.LittleEndian.Uint64(data[1:9]),
		}, true, nil
	case 12: // transferChecked
		if len(data) < 1+8+1 {
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
			amount:      binary.LittleEndian.Uint64(data[1:9]),
			decimals:    data[9],
			hasDecimals: true,
		}, true, nil
	case 7: // mintTo
		if len(data) < 1+8 {
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
			amount:  binary.LittleEndian.Uint64(data[1:9]),
		}, true, nil
	case 14: // mintToChecked
		if len(data) < 1+8+1 {
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
			amount:      binary.LittleEndian.Uint64(data[1:9]),
			decimals:    data[9],
			hasDecimals: true,
		}, true, nil
	case 8: // burn
		if len(data) < 1+8 {
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
			amount:  binary.LittleEndian.Uint64(data[1:9]),
		}, true, nil
	case 15: // burnChecked
		if len(data) < 1+8+1 {
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
			amount:      binary.LittleEndian.Uint64(data[1:9]),
			decimals:    data[9],
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
