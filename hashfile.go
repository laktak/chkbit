package chkbit

import (
	"crypto/md5"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"

	"lukechampine.com/blake3"
)

const BLOCKSIZE = 2 << 10 << 7 // kb

func Hashfile(path string, hashAlgo string, perfMonBytes func(int64)) (string, int64, int64, error) {
	var h hash.Hash
	switch hashAlgo {
	case "md5":
		h = md5.New()
	case "sha512":
		h = sha512.New()
	case "blake3":
		h = blake3.New(32, nil)
	default:
		return "", 0, 0, errors.New("algo '" + hashAlgo + "' is unknown.")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer file.Close()

	// Get file info AFTER opening to avoid race condition
	var info os.FileInfo
	if info, err = file.Stat(); err != nil {
		return "", 0, 0, err
	}
	mtime := int64(info.ModTime().UnixNano() / 1e6) // convert to ms
	size := info.Size()

	buf := make([]byte, BLOCKSIZE)
	for {
		bytesRead, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return "", 0, 0, err
		}
		if bytesRead == 0 {
			break
		}
		h.Write(buf[:bytesRead])
		if perfMonBytes != nil {
			perfMonBytes(int64(bytesRead))
		}
	}
	return hex.EncodeToString(h.Sum(nil)), mtime, size, nil
}

func hashMd5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
