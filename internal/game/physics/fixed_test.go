package physics

import "testing"

// mm8: 0x451690 (Math_Isqrt). Traced by hand: the digit loop gives q = floor(sqrt v),
// then q+1 when v - q*q >= q - 1, so the rounding is biased up (sqrt 5 -> 3).
func TestIsqrt(t *testing.T) {
	for _, c := range [][2]uint32{
		{0, 0}, {1, 1}, {2, 2}, {3, 2}, {4, 2}, {5, 3}, {8, 3}, {9, 3}, {15, 4}, {16, 4},
		{400, 20}, {401, 20}, {418, 20}, {419, 21}, {384 * 384, 384},
		{0x7fffffff, 46341},
	} {
		if got := Isqrt(c[0]); got != c[1] {
			t.Errorf("Isqrt(%d) = %d, want %d", c[0], got, c[1])
		}
	}
	// int32-negative inputs come back unchanged (the CMP ECX,1 / JG is signed).
	if got := Isqrt(0x80000000); got != 0x80000000 {
		t.Errorf("Isqrt(0x80000000) = %#x", got)
	}
}

// mm8: 0x45155b (MathTables_Init: tan/cos/sec(i*pi/1024)*65536 + 0.5), 0x402e04
// (Math_Cos: mirror at the half turn, negate past the quarter)
func TestCos(t *testing.T) {
	for _, c := range [][2]int32{
		{0, 0x10000}, {256, 46341}, {512, 0}, {768, -46341}, {1024, -0x10000},
		{1280, -46341}, {1536, 0}, {1792, 46341}, {2048, 0x10000}, {-256, 46341},
		{1, 65536}, // cos(pi/1024)*65536 = 65535.69, + 0.5 truncated
		{100, 62476},
	} {
		if got := Cos(c[0]); got != c[1] {
			t.Errorf("Cos(%d) = %d, want %d", c[0], got, c[1])
		}
	}
	if got := Sin(512); got != 0x10000 {
		t.Errorf("Sin(512) = %d", got)
	}
	if tanTable[256] != 0x10000 || tanTable[0] != 0 || tanTable[0x200] != -0x10000001 {
		t.Errorf("tan[0], tan[256], tan[512] = %d, %d, %#x", tanTable[0], tanTable[256], uint32(tanTable[0x200]))
	}
	if secTable[0] != 0x10000 || cosTable[0x200] != 0 {
		t.Errorf("sec[0] %d cos[512] %d", secTable[0], cosTable[0x200])
	}
}

// mm8: 0x4513ec (Math_Atan2). The linear search stops one short of the binary search's
// upper bound, so exactly 45 degrees comes out as 255.
func TestAtan2(t *testing.T) {
	for _, c := range [][3]int32{
		{100, 0, 0}, {0, 100, 512}, {-100, 0, 1024}, {0, -100, 1536}, {0, 0, 1536},
		{100, 100, 255}, {-100, 100, 1024 - 255}, {-100, -100, 1024 + 255}, {100, -100, 2048 - 255},
		// The result is the first index past the binary search whose tangent exceeds y/x:
		// y/x = 65/65536 is below tan[1] = 201, giving 1; 37814 lies between tan[170] =
		// 37637 and tan[171] = 37919, giving 171.
		{1000, 1, 1},
		{1000, 577, 171},
	} {
		if got := Atan2(c[0], c[1]); got != c[2] {
			t.Errorf("Atan2(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}

func TestMulDiv16(t *testing.T) {
	if Mul16(-1, 1) != -1 || Mul16(0x10000, 0x10000) != 0x10000 || Mul16(3<<16, -2<<16) != -6<<16 {
		t.Error("Mul16")
	}
	if Div16(1, 2) != 0x8000 || Div16(-63<<16, -0x10000) != 63<<16 || Div16(-1, 3) != -21845 {
		t.Error("Div16")
	}
}
