package core

import (
	"unicode/utf8"
)

// Controlled construction selects pieces after the charged decoding pass,
// avoiding an uninterruptible rune-count scan merely to choose storage.
func NewControlledDocumentInput(text string, unit PositionUnit, c *BudgetControl) Input {
	if !BudgetControlsEnabled || c == nil {
		if UseDocumentPieces(text, unit) {
			return Input{Unit: unit, Eof: true, BaseLine: 1, BaseCol: 1, Pieces: NewDocumentPieces(text, unit)}
		}
		return newTextInputIn(text, unit, true, nil)
	}
	if DocumentPieceInput && unit == Bytes && len(text) >= DocumentPieceMinSize {
		// Immutable byte pieces retain the supplied string without copying it.
		return Input{Unit: unit, Eof: true, BaseLine: 1, BaseCol: 1, Pieces: NewDocumentPieces(text, unit)}
	}
	in := NewControlledTextInput(text, unit, true, nil, c)
	if !DocumentPieceInput || in.Loaded() < DocumentPieceMinSize {
		return in
	}
	piece := DocumentPiece{Text: text, Runes: in.In, Offs: in.Offs}
	if !in.SrcOK {
		piece.Text = in.detachedText(0, len(in.In))
		piece.Offs = nil
		if int64(len(piece.Text)) > 1<<31-1 {
			piece.LargeOffs = make([]int, len(in.In)+1)
		} else {
			piece.Offs = make([]int32, len(in.In)+1)
		}
		offset := 0
		for i, r := range in.In {
			in.spendRead(1)
			if piece.LargeOffs != nil {
				piece.LargeOffs[i] = offset
			} else {
				piece.Offs[i] = int32(offset)
			}
			offset += utf8.RuneLen(r)
		}
		if piece.LargeOffs != nil {
			piece.LargeOffs[len(in.In)] = offset
		} else {
			piece.Offs[len(in.In)] = int32(offset)
		}
	}
	in.Pieces = &documentPieces{unit: unit, Parts: []DocumentPiece{piece}, N: len(in.In)}
	in.In, in.Offs, in.Src, in.SrcOK = nil, nil, "", false
	return in
}

// Text across immutable pieces copies or flattens as documentPieces.Text
// does, charging each bounded step; an interrupted step publishes nothing.
func (in *Input) controlledPieceText(start, end int) string {
	d := in.Pieces
	if start == end {
		return ""
	}
	if !d.FullOK {
		i, off := d.find(start)
		if piece := d.Parts[i]; end <= piece.start+piece.length(d.unit) {
			return piece.Substring(off, off+end-start, d.unit)
		}
		return d.crossingText(i, start, end, in.Control.Charge)
	}
	return d.fullText(start, end, in.Control.Charge)
}
