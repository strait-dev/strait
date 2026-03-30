/**
 * This function that will receive a string as an argument, replace the comma with a dot, and then convert the string to a float.
 * It is going to be used to convert the string that comes from the number input to a float number.
 * @param input {string} - The string to be converted
 * @returns number - The converted number
 */
export const convertStringToFloat = (input: string): number => {
  const replacedString = input.replace(",", ".");
  const floatNumber = Number.parseFloat(replacedString);
  if (Number.isNaN(floatNumber)) {
    return 0;
  }
  return floatNumber;
};

// TypeScript array find possible undefined
// I got the solution from the following link:
// https://stackoverflow.com/questions/54738221/typescript-array-find-possibly-undefined
/**
 * TypeScript array find possible undefined
 * I got the solution from the following link:
 * https://stackoverflow.com/questions/54738221/typescript-array-find-possibly-undefined
 * @param array {T[]} - The array to be checked
 * @param message {string} - The message to be thrown if the array is undefined
 * @returns T - The array
 */
export function ensure<T>(
  argument: T | undefined | null,
  message = "This value was promised to be there."
): T {
  if (argument === undefined || argument === null) {
    throw new TypeError(message);
  }

  return argument;
}

