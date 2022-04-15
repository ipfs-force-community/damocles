package util

import (
	"fmt"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/ipfs/go-cid"
)

func ReplicaCommitment2CID(commR [32]byte) (cid.Cid, error) {
	return commcid.ReplicaCommitmentV1ToCID(commR[:])
}

func CID2ReplicaCommitment(sealedCID cid.Cid) ([32]byte, error) {
	var commR [32]byte

	if !sealedCID.Defined() {
		return commR, fmt.Errorf("undefined cid")
	}

	b, err := commcid.CIDToReplicaCommitmentV1(sealedCID)
	if err != nil {
		return commR, fmt.Errorf("convert to commitment: %w", err)
	}

	if size := len(b); size != 32 {
		return commR, fmt.Errorf("get %d bytes for commitment", size)
	}

	copy(commR[:], b[:])
	return commR, nil
}

func DataCommitment2CID(commD [32]byte) (cid.Cid, error) {
	return commcid.DataCommitmentV1ToCID(commD[:])
}

func CID2DataCommitment(unsealedCID cid.Cid) ([32]byte, error) {
	var commD [32]byte

	if !unsealedCID.Defined() {
		return commD, fmt.Errorf("undefined cid")
	}

	b, err := commcid.CIDToDataCommitmentV1(unsealedCID)
	if err != nil {
		return commD, fmt.Errorf("convert to commitment: %w", err)
	}

	if size := len(b); size != 32 {
		return commD, fmt.Errorf("get %d bytes for commitment", size)
	}

	copy(commD[:], b[:])
	return commD, nil
}
