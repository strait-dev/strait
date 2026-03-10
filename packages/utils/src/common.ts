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

/**
 * Get the URL of the current site. Used for Next.js server side rendering.
 * @returns The URL of the current site.
 */
export function getURL() {
  let url =
    process?.env?.NEXT_PUBLIC_SITE_URL ?? // Set this to your site URL in production env.
    process?.env?.NEXT_PUBLIC_VERCEL_URL ?? // Automatically set by Vercel.
    "http://localhost:3000/";
  // Make sure to include `https://` when not localhost.
  url = url.includes("http") ? url : `https://${url}`;
  // Make sure to including trailing `/`.
  url = url.at(-1) === "/" ? url : `${url}/`;
  return url;
}

/**
 * Helper function to handle the promise for toast.
 * It is going to be used to handle the promise for toast.
 * We use this because doesn't have a way to show error in promise toast with server actions.
 * Link: https://github.com/emilkowalski/sonner/issues/450#issuecomment-2338874682
 * @param promise {Promise<{ success: boolean; data: any }>} - The promise to be handled
 * @returns {Promise<any>} - The result of the promise
 */
export const generatePromise = async <_T>(
  promise: Promise<{ success: boolean; data: any }>
): Promise<any> => {
  const result = await promise;
  if (result.success) {
    return result.data;
  }
  throw result.data;
};
