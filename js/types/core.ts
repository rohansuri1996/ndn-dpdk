/**
 * Non-negative counter.
 * It may be encoded as integer (uint32 or shorter) or string (uint64).
 */
export type Counter = number|string;

/**
 * @TJS-type integer
 * @minimum 0
 */
export type Index = number;

/**
 * Base64 encoded binary blob.
 * @TJS-contentEncoding base64
 * @TJS-contentMediaType application/octet-stream
 */
export type Blob = string;

/**
 * @TJS-type integer
 * @minimum 0
 */
type NNDurationNumber = number;

type NNDuration = NNDurationNumber | string;

/**
 * Non-negative duration in milliseconds.
 * This can be either a number in milliseconds, or a string with any valid duration unit.
 */
export type NNMilliseconds = NNDuration;

/**
 * Non-negative duration in nanoseconds.
 * This can be either a number in nanoseconds, or a string with any valid duration unit.
 */
export type NNNanoseconds = NNDuration;

/** Snapshot from runningstat. */
export interface RunningStatSnapshot {
  /**
   * Number of inputs.
   * @TJS-type integer
   * @minimum 0
   */
  count: number;

  /**
   * Number of samples.
   * @TJS-type integer
   * @minimum 0
   */
  len: number;

  /** Minimum value. */
  min?: number;

  /** Maximum value. */
  max?: number;

  /** Mean. */
  mean?: number;

  /**
   * Variance of samples.
   * @minimum 0
   */
  variance?: number;

  /**
   * Standard deviation of samples.
   * @minimum 0
   */
  stdev?: number;

  /** Internal variable M1. */
  m1: number;

  /** Internal variable M2. */
  m2: number;
}

/** Name represented as canonical URI. */
export type Name = string;
