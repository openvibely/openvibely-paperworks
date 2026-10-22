package xlsx

// walkCoordinates visits every coordinate in the inclusive rectangle in
// row-major order. Callers are responsible for validating and ordering bounds.
func walkCoordinates(left, top, right, bottom int, visit func(col, row int) error) error {
	for row := top; row <= bottom; row++ {
		for col := left; col <= right; col++ {
			if err := visit(col, row); err != nil {
				return err
			}
		}
	}
	return nil
}
