package main

// main is the entry point for the MySQLight REPL. It reads the database file path
// from the command-line arguments, then opens or creates the database using the Go API.
// After printing a welcome banner, it enters a loop that reads SQL statements line by line,
// passes them to the executor, and prints results or errors. The built-in commands .exit
// and .quit close the database cleanly and terminate the process.
func main() {
	// TODO: read the database file path from os.Args[1]
	// TODO: open or create the database via the Go API (mysqlight.Open)
	// TODO: print a welcome banner
	// TODO: start the REPL loop:
	//   - print "MySQLight> " prompt
	//   - read a line of input (bufio.Scanner)
	//   - handle built-in commands: .exit / .quit → close db and os.Exit(0)
	//   - pass everything else to the executor, print results or errors
}
