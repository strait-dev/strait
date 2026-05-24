declare module "@better-auth/passkey/client" {
  /**
   * Type bridge for the Better Auth passkey client subpath.
   *
   * The package exposes `./client` through package exports and Vite resolves it
   * correctly at build time, but `tsgo` does not currently resolve the subpath
   * declaration from this package.
   */
  export { passkeyClient } from "../../node_modules/@better-auth/passkey/dist/client.mjs";
}
