package cli

const (
	// ExitSuccess indicates successful execution
	ExitSuccess = 0

	// ExitGeneralError indicates a general application error
	ExitGeneralError = 1

	// ExitAWSError indicates an error from AWS API calls
	ExitAWSError = 2

	// ExitInvalidInput indicates invalid user input or arguments
	ExitInvalidInput = 3
)
