import { nanoid } from "nanoid";

/**
 * Create a unique id for a skeleton.
 * We use nanoid to generate a unique id.
 * This is to avoid the use of Math.random() which is not cryptographically secure.
 * @param prefix - The prefix for the skeleton id.
 * @returns The unique id for the skeleton.
 */
export const createSkeletonId = (prefix: string) => `${prefix}-${nanoid()}`;
