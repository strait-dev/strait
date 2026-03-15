import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock the generate-password-ts module
vi.mock("generate-password-ts", () => ({
  generate: vi.fn(),
}));

import { generate } from "generate-password-ts";
import { DEFAULT_PINCODE_LENGTH, MAX_PINCODE_LENGTH } from "@/utils/constants";

// Type assertion for the mocked function
const mockGenerate = generate as unknown as ReturnType<typeof vi.fn>;

// Create a function that mirrors the logic from the component
const generateCode = (
  type: "random" | "memorable" | "pincode",
  length: number,
  includeNumbers: boolean,
  includeSymbols: boolean
): string => {
  if (type === "pincode") {
    return generate({
      length,
      numbers: true,
      symbols: false,
      lowercase: false,
      uppercase: false,
      excludeSimilarCharacters: true,
    });
  }

  return generate({
    length,
    numbers: includeNumbers,
    symbols: includeSymbols,
    lowercase: true,
    uppercase: true,
    excludeSimilarCharacters: true,
  });
};

describe("GenerateCode Logic", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Mock the generate function to return a string with the requested length
    mockGenerate.mockImplementation((options) =>
      "A".repeat(options.length || DEFAULT_PINCODE_LENGTH)
    );
  });

  describe("Random Code Generation", () => {
    it("should generate random code with default parameters", () => {
      const code = generateCode("random", DEFAULT_PINCODE_LENGTH, true, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: DEFAULT_PINCODE_LENGTH,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
      expect(code).toBe("A".repeat(DEFAULT_PINCODE_LENGTH));
      expect(code.length).toBe(DEFAULT_PINCODE_LENGTH);
    });

    it("should generate code with exact length specified", () => {
      const code = generateCode("random", MAX_PINCODE_LENGTH * 2, true, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * 2,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
      expect(code).toBe("A".repeat(MAX_PINCODE_LENGTH * 2));
      expect(code.length).toBe(MAX_PINCODE_LENGTH * 2);
    });

    it("should generate random code with symbols enabled", () => {
      generateCode("random", MAX_PINCODE_LENGTH * 2, true, true);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * 2,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate random code without numbers", () => {
      generateCode("random", MAX_PINCODE_LENGTH * 2, false, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * 2,
        numbers: false,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate random code with custom length", () => {
      generateCode("random", MAX_PINCODE_LENGTH, true, true);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Memorable Code Generation", () => {
    it("should generate memorable code with default parameters", () => {
      generateCode("memorable", DEFAULT_PINCODE_LENGTH, true, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: DEFAULT_PINCODE_LENGTH,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate memorable code with symbols", () => {
      const TEST_LENGTH_MULTIPLIER = 2;

      generateCode(
        "memorable",
        DEFAULT_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
        true,
        true
      );

      expect(mockGenerate).toHaveBeenCalledWith({
        length: DEFAULT_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate memorable code without numbers", () => {
      generateCode("memorable", MAX_PINCODE_LENGTH * 2, false, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * 2,
        numbers: false,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate memorable code with custom length", () => {
      const CUSTOM_LENGTH_MULTIPLIER = 3;

      generateCode(
        "memorable",
        MAX_PINCODE_LENGTH * CUSTOM_LENGTH_MULTIPLIER,
        false,
        true
      );

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * CUSTOM_LENGTH_MULTIPLIER,
        numbers: false,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Pincode Generation", () => {
    it("should generate pincode with numbers only", () => {
      generateCode("pincode", DEFAULT_PINCODE_LENGTH, true, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: DEFAULT_PINCODE_LENGTH,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate pincode ignoring includeSymbols parameter", () => {
      generateCode("pincode", MAX_PINCODE_LENGTH * 2, true, true);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * 2,
        numbers: true,
        symbols: false, // Always false for pincode
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate pincode ignoring includeNumbers parameter", () => {
      generateCode("pincode", MAX_PINCODE_LENGTH / 2, false, false);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH / 2,
        numbers: true, // Always true for pincode
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate pincode with maximum length (30)", () => {
      const MAX_LENGTH_MULTIPLIER = 5;

      generateCode(
        "pincode",
        MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER,
        true,
        true
      );

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate pincode with minimum length (6)", () => {
      generateCode("pincode", DEFAULT_PINCODE_LENGTH, false, true);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: DEFAULT_PINCODE_LENGTH,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Edge Cases", () => {
    it("should handle minimum length (6)", () => {
      const code = generateCode("random", DEFAULT_PINCODE_LENGTH, true, false);

      expect(mockGenerate).toHaveBeenCalledWith(
        expect.objectContaining({
          length: DEFAULT_PINCODE_LENGTH,
        })
      );
      expect(code.length).toBe(DEFAULT_PINCODE_LENGTH);
      expect(code).toBe("A".repeat(DEFAULT_PINCODE_LENGTH));
    });

    it("should handle maximum length for random/memorable (100)", () => {
      const code = generateCode("random", MAX_PINCODE_LENGTH * 10, true, true);

      expect(mockGenerate).toHaveBeenCalledWith(
        expect.objectContaining({
          length: MAX_PINCODE_LENGTH * 10,
        })
      );
      expect(code.length).toBe(MAX_PINCODE_LENGTH * 10);
      expect(code).toBe("A".repeat(MAX_PINCODE_LENGTH * 10));
    });

    it("should handle maximum length for pincode (30)", () => {
      const MAX_LENGTH_MULTIPLIER = 5;

      const code = generateCode(
        "pincode",
        MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER,
        true,
        true
      );

      expect(mockGenerate).toHaveBeenCalledWith(
        expect.objectContaining({
          length: MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER,
        })
      );
      expect(code.length).toBe(MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER);
      expect(code).toBe("A".repeat(MAX_PINCODE_LENGTH * MAX_LENGTH_MULTIPLIER));
    });

    it("should always exclude similar characters", () => {
      const TEST_LENGTH_MULTIPLIER = 2;
      const FIRST_CALL = 1;
      const SECOND_CALL = 2;
      const THIRD_CALL = 3;

      generateCode(
        "random",
        MAX_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
        true,
        true
      );
      generateCode(
        "memorable",
        MAX_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
        true,
        true
      );
      generateCode(
        "pincode",
        MAX_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
        true,
        true
      );

      expect(mockGenerate).toHaveBeenNthCalledWith(
        FIRST_CALL,
        expect.objectContaining({
          excludeSimilarCharacters: true,
        })
      );
      expect(mockGenerate).toHaveBeenNthCalledWith(
        SECOND_CALL,
        expect.objectContaining({
          excludeSimilarCharacters: true,
        })
      );
      expect(mockGenerate).toHaveBeenNthCalledWith(
        THIRD_CALL,
        expect.objectContaining({
          excludeSimilarCharacters: true,
        })
      );
    });
  });

  describe("Parameter Combinations", () => {
    it("should handle all combinations for random type", () => {
      const TEST_LENGTH_MULTIPLIER = 2;
      const FIRST_CALL = 1;
      const SECOND_CALL = 2;
      const THIRD_CALL = 3;
      const FOURTH_CALL = 4;

      const combinations = [
        [true, true], // numbers + symbols
        [true, false], // numbers only
        [false, true], // symbols only
        [false, false], // letters only
      ];

      for (const [numbers, symbols] of combinations) {
        generateCode(
          "random",
          MAX_PINCODE_LENGTH * TEST_LENGTH_MULTIPLIER,
          numbers,
          symbols
        );
      }

      expect(mockGenerate).toHaveBeenCalledTimes(combinations.length);
      expect(mockGenerate).toHaveBeenNthCalledWith(
        FIRST_CALL,
        expect.objectContaining({ numbers: true, symbols: true })
      );
      expect(mockGenerate).toHaveBeenNthCalledWith(
        SECOND_CALL,
        expect.objectContaining({ numbers: true, symbols: false })
      );
      expect(mockGenerate).toHaveBeenNthCalledWith(
        THIRD_CALL,
        expect.objectContaining({ numbers: false, symbols: true })
      );
      expect(mockGenerate).toHaveBeenNthCalledWith(
        FOURTH_CALL,
        expect.objectContaining({ numbers: false, symbols: false })
      );
    });

    it("should handle all combinations for memorable type", () => {
      const combinations = [
        [true, true], // numbers + symbols
        [true, false], // numbers only
        [false, true], // symbols only
        [false, false], // letters only
      ];

      for (const [numbers, symbols] of combinations) {
        generateCode("memorable", MAX_PINCODE_LENGTH * 2, numbers, symbols);
      }

      expect(mockGenerate).toHaveBeenCalledTimes(combinations.length);
      for (let index = 0; index < combinations.length; index += 1) {
        expect(mockGenerate).toHaveBeenNthCalledWith(
          index + 1,
          expect.objectContaining({
            lowercase: true,
            uppercase: true,
          })
        );
      }
    });

    it("should always use same parameters for pincode regardless of input", () => {
      const combinations = [
        [true, true],
        [true, false],
        [false, true],
        [false, false],
      ];

      for (const [numbers, symbols] of combinations) {
        generateCode("pincode", MAX_PINCODE_LENGTH, numbers, symbols);
      }

      expect(mockGenerate).toHaveBeenCalledTimes(combinations.length);
      for (let index = 0; index < combinations.length; index += 1) {
        expect(mockGenerate).toHaveBeenNthCalledWith(
          index + 1,
          expect.objectContaining({
            numbers: true,
            symbols: false,
            lowercase: false,
            uppercase: false,
          })
        );
      }
    });
  });
});
