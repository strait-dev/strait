package domain

// RunTokenIssuer is the JWT issuer used for SDK run tokens minted for a
// specific job run. Verifiers must reject run tokens with any other issuer.
const RunTokenIssuer = "strait:run-token"
