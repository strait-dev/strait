import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock the generate-password-ts module
vi.mock("generate-password-ts", () => ({
  generate: vi.fn(),
}));

import { generate } from "generate-password-ts";

// Type assertion for the mocked function
const mockGenerate = generate as unknown as ReturnType<typeof vi.fn>;

const MAX_CHARACTERS = 100;
const MIN_CHARACTERS = 6;
const PINCODE_MAX_CHARACTERS = 30;

/**
 * Generate Code Dialog Form Logic Tests
 *
 * Since the app doesn't use React Testing Library, these tests focus on
 * testing the form logic, validation, and code generation behavior rather
 * than UI interactions.
 */

// Mock form state to simulate React Hook Form behavior
type FormState = {
  type: "random" | "memorable" | "pincode";
  characters: number;
  include_numbers: boolean;
  include_symbols: boolean;
  code: string;
};

// Helper function to simulate the component's generateCode logic
const generateCodeLogic = (formState: FormState): string => {
  const { type, characters, include_numbers, include_symbols } = formState;

  if (type === "pincode") {
    return generate({
      length: characters,
      numbers: true,
      symbols: false,
      lowercase: false,
      uppercase: false,
      excludeSimilarCharacters: true,
    });
  }

  return generate({
    length: characters,
    numbers: include_numbers,
    symbols: include_symbols,
    lowercase: true,
    uppercase: true,
    excludeSimilarCharacters: true,
  });
};

// Helper function to simulate form validation
const validateForm = (formState: FormState): boolean =>
  formState.code.length > 0 && formState.characters >= MIN_CHARACTERS;

// Helper function to simulate max value calculation
const getMaxCharacters = (type: "random" | "memorable" | "pincode"): number =>
  type === "pincode" ? PINCODE_MAX_CHARACTERS : MAX_CHARACTERS;

describe("GenerateCodeDialog Form Logic", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Mock the generate function to return a string with the requested length
    mockGenerate.mockImplementation((options) =>
      "A".repeat(options.length || MIN_CHARACTERS)
    );
  });

  describe("Default Form State", () => {
    it("should have correct default values", () => {
      const defaultState: FormState = {
        type: "random",
        characters: MIN_CHARACTERS,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      expect(defaultState.type).toBe("random");
      expect(defaultState.characters).toBe(MIN_CHARACTERS);
      expect(defaultState.include_numbers).toBe(true);
      expect(defaultState.include_symbols).toBe(false);
      expect(defaultState.code).toBe("");
    });

    it("should generate initial code with default settings", () => {
      mockGenerate.mockReturnValue("INITIAL_CODE");

      const formState: FormState = {
        type: "random",
        characters: MIN_CHARACTERS,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      const code = generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MIN_CHARACTERS,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
      expect(code).toBe("INITIAL_CODE");
    });
  });

  describe("Type Changes", () => {
    it("should adjust max characters when changing to pincode", () => {
      expect(getMaxCharacters("random")).toBe(MAX_CHARACTERS);
      expect(getMaxCharacters("memorable")).toBe(MAX_CHARACTERS);
      expect(getMaxCharacters("pincode")).toBe(PINCODE_MAX_CHARACTERS);
    });

    it("should limit characters to 30 when type is pincode", () => {
      const formState: FormState = {
        type: "pincode",
        characters: PINCODE_MAX_CHARACTERS + 1, // Over the limit
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      const maxChars = getMaxCharacters(formState.type);
      const limitedChars = Math.min(formState.characters, maxChars);

      expect(limitedChars).toBe(PINCODE_MAX_CHARACTERS);
    });

    it("should generate pincode with numbers only", () => {
      mockGenerate.mockReturnValue("123456");

      const formState: FormState = {
        type: "pincode",
        characters: MIN_CHARACTERS,
        include_numbers: false, // Should be ignored for pincode
        include_symbols: true, // Should be ignored for pincode
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 6,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate memorable code with user preferences", () => {
      mockGenerate.mockReturnValue("MemorableWord123");

      const formState: FormState = {
        type: "memorable",
        characters: 8,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 8,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Form Validation", () => {
    it("should validate form correctly with valid code", () => {
      const validFormState: FormState = {
        type: "random",
        characters: 8,
        include_numbers: true,
        include_symbols: false,
        code: "ValidCode123",
      };

      expect(validateForm(validFormState)).toBe(true);
    });

    it("should invalidate form with empty code", () => {
      const invalidFormState: FormState = {
        type: "random",
        characters: 8,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      expect(validateForm(invalidFormState)).toBe(false);
    });

    it("should invalidate form with characters less than 6", () => {
      const invalidFormState: FormState = {
        type: "random",
        characters: 5,
        include_numbers: true,
        include_symbols: false,
        code: "ABC12",
      };

      expect(validateForm(invalidFormState)).toBe(false);
    });
  });

  describe("Code Generation with Different Parameters", () => {
    it("should generate code with symbols enabled", () => {
      mockGenerate.mockReturnValue("Code@123!");

      const formState: FormState = {
        type: "random",
        characters: 8,
        include_numbers: true,
        include_symbols: true,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 8,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate code without numbers", () => {
      mockGenerate.mockReturnValue("CodeOnly");

      const formState: FormState = {
        type: "random",
        characters: 8,
        include_numbers: false,
        include_symbols: false,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 8,
        numbers: false,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should generate longer codes correctly", () => {
      mockGenerate.mockReturnValue("VeryLongCodeWith15Characters");

      const formState: FormState = {
        type: "random",
        characters: 15,
        include_numbers: true,
        include_symbols: true,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 15,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Edge Cases", () => {
    it("should handle minimum character length", () => {
      mockGenerate.mockReturnValue("Min6Ch");

      const formState: FormState = {
        type: "random",
        characters: 6,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 6,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });

    it("should handle maximum character length for pincode", () => {
      mockGenerate.mockReturnValue("123456789012345678901234567890");

      const formState: FormState = {
        type: "pincode",
        characters: 30,
        include_numbers: true,
        include_symbols: true,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: 30,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should handle maximum character length for random/memorable", () => {
      mockGenerate.mockReturnValue("A".repeat(MAX_CHARACTERS));

      const formState: FormState = {
        type: "random",
        characters: MAX_CHARACTERS,
        include_numbers: true,
        include_symbols: true,
        code: "",
      };

      generateCodeLogic(formState);

      expect(mockGenerate).toHaveBeenCalledWith({
        length: MAX_CHARACTERS,
        numbers: true,
        symbols: true,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });

  describe("Form State Changes", () => {
    it("should regenerate code when type changes", () => {
      mockGenerate
        .mockReturnValueOnce("RandomCode123")
        .mockReturnValueOnce("123456");

      // Initial random generation
      const initialState: FormState = {
        type: "random",
        characters: MIN_CHARACTERS,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      generateCodeLogic(initialState);

      // Change to pincode
      const updatedState: FormState = {
        ...initialState,
        type: "pincode",
      };

      generateCodeLogic(updatedState);

      expect(mockGenerate).toHaveBeenCalledTimes(2);
      expect(mockGenerate).toHaveBeenNthCalledWith(1, {
        length: MIN_CHARACTERS,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
      expect(mockGenerate).toHaveBeenNthCalledWith(2, {
        length: MIN_CHARACTERS,
        numbers: true,
        symbols: false,
        lowercase: false,
        uppercase: false,
        excludeSimilarCharacters: true,
      });
    });

    it("should regenerate code when characters change", () => {
      mockGenerate
        .mockReturnValueOnce("Short6")
        .mockReturnValueOnce("LongerCode10");

      // Initial generation with 6 characters
      const initialState: FormState = {
        type: "random",
        characters: MIN_CHARACTERS,
        include_numbers: true,
        include_symbols: false,
        code: "",
      };

      generateCodeLogic(initialState);

      // Change to 10 characters
      const updatedState: FormState = {
        ...initialState,
        characters: 10,
      };

      generateCodeLogic(updatedState);

      expect(mockGenerate).toHaveBeenCalledTimes(2);
      expect(mockGenerate).toHaveBeenNthCalledWith(1, {
        length: MIN_CHARACTERS,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
      expect(mockGenerate).toHaveBeenNthCalledWith(2, {
        length: 10,
        numbers: true,
        symbols: false,
        lowercase: true,
        uppercase: true,
        excludeSimilarCharacters: true,
      });
    });
  });
});
