package bittorrent

type PieceMap struct {
	pieces []*PieceMapPiece
}

func NewPieceMap(pieceCount int) *PieceMap {

	pieces := make([]*PieceMapPiece, 0, pieceCount)
	for i := range pieces {
		pieces[i] = &PieceMapPiece{

		}
	}

	return &PieceMap {
		pieces : pieces,
	}
}

func (pm * PieceMap) Inc(index uint32) {

}
func (pm * PieceMap) IncBitfield(bits []byte) {

}
func (pm * PieceMap) Dec(index uint32) {

}
func (pm * PieceMap) DecBitfield(bits []byte) {

}
func (pm * PieceMap) IsValid(index, begin, length uint32) bool {
	return true
}
func (pm * PieceMap) Piece(index uint32) *PieceMapPiece {
	return pm.pieces[index]
}


type PieceMapPiece struct {

}

func (pmp * PieceMapPiece) BlocksNeeded() bool {
	return true
}
