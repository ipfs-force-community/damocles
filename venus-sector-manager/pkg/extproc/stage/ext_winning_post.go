package stage

const NameWinningPost = "winning_post"

type WinningPost = WindowPoSt

type WinningPoStOutput struct {
	Proofs [][]byte `json:"proofs"`
}
