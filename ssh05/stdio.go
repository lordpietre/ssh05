/*
MIT License

Copyright (c) 2023-2026 The Trzsz SSH Authors.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package ssh05

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var pauseOutput atomic.Bool
var outputWaitGroup sync.WaitGroup

var (
	startBracket = []byte{0x1b, '[', '2', '0', '0', '~'}
	endBracket   = []byte{0x1b, '[', '2', '0', '1', '~'}
)

func writeAll(dst io.Writer, data []byte) error {
	m := 0
	l := len(data)
	for m < l {
		n, err := dst.Write(data[m:])
		if err != nil {
			return err
		}
		m += n
	}
	return nil
}

func forwardInput(reader io.Reader, writer io.WriteCloser, win bool, escapeChar byte, escapeTime time.Duration, sshConn *sshConnection) {
	defer func() {
		_ = writer.Close()
		debug("ssh session stdin forward completed")
	}()

	if sshConn.param.args.NoStdin {
		return
	}

	var enterPressedFlag bool
	var enterPressedTime time.Time
	var inBracketedPaste bool
	var pasteBuf bytes.Buffer

	buffer := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if enableDebugLogging {
				if ch := stdinInputChan.Load(); ch != nil {
					*ch <- append([]byte(nil), buffer[:n]...)
					continue
				}
			}

			data := buffer[:n]

			// handle bracketed paste mode
			if inBracketedPaste {
				if idx := bytes.Index(data, endBracket); idx >= 0 {
					if idx > 0 {
						pasteBuf.Write(data[:idx])
					}
					if pasteBuf.Len() > 0 {
						if err := writeAll(writer, pasteBuf.Bytes()); err != nil {
							if !sshConn.closed.Load() {
								warning("wrap input write failed: %v", err)
							}
							return
						}
						pasteBuf.Reset()
					}
					inBracketedPaste = false
					rest := data[idx+len(endBracket):]
					if len(rest) > 0 {
						data = rest
					} else {
						continue
					}
				} else {
					pasteBuf.Write(data)
					continue
				}
			}

			if !inBracketedPaste {
				if idx := bytes.Index(data, startBracket); idx >= 0 {
					if idx > 0 {
						if err := writeForwardInput(data[:idx], writer, win, sshConn, escapeTime, &enterPressedFlag, &enterPressedTime, escapeChar); err != nil {
							return
						}
					}
					inBracketedPaste = true
					pasteBuf.Reset()
					inner := data[idx+len(startBracket):]
					if len(inner) > 0 {
						if endIdx := bytes.Index(inner, endBracket); endIdx >= 0 {
							if endIdx > 0 {
								if err := writeAll(writer, inner[:endIdx]); err != nil {
									if !sshConn.closed.Load() {
										warning("wrap input write failed: %v", err)
									}
									return
								}
							}
							inBracketedPaste = false
							rest := inner[endIdx+len(endBracket):]
							if len(rest) > 0 {
								data = rest
							} else {
								continue
							}
						} else {
							pasteBuf.Write(inner)
							continue
						}
					} else {
						continue
					}
				}
			}

			if err := writeForwardInput(data, writer, win, sshConn, escapeTime, &enterPressedFlag, &enterPressedTime, escapeChar); err != nil {
				return
			}
		}
		if err == io.EOF {
			if win && isTerminal && sshConn.tty {
				_, _ = writer.Write([]byte{0x1A})  // ctrl + z
				time.Sleep(100 * time.Millisecond) // give it a break just in case of real EOF
				continue
			}
			return
		}
		if err != nil {
			return
		}
	}
}

func writeForwardInput(data []byte, writer io.WriteCloser, win bool, sshConn *sshConnection, escapeTime time.Duration, enterPressedFlag *bool, enterPressedTime *time.Time, escapeChar byte) error {
	if escapeTime > 0 {
		if len(data) == 1 && data[0] == '\r' {
			*enterPressedFlag = true
			*enterPressedTime = time.Now()
		} else if *enterPressedFlag && len(data) == 1 && data[0] == escapeChar && time.Since(*enterPressedTime) <= escapeTime {
			pauseOutput.Store(true)
			runConsole(escapeChar, writer, sshConn)
			pauseOutput.Store(false)
			return nil
		} else {
			*enterPressedFlag = false
		}
	}

	buf := data
	if win && !sshConn.tty {
		buf = bytes.ReplaceAll(buf, []byte("\r\n"), []byte("\n"))
	}
	if err := writeAll(writer, buf); err != nil {
		if !sshConn.closed.Load() {
			warning("wrap input write failed: %v", err)
		}
		return err
	}
	return nil
}

func forwardOutput(reader io.Reader, writer io.WriteCloser, win, tty bool) {
	// Don't close os.Stdout and os.Stderr here.
	// There may be some debugging message that needs to be output.
	// The process is about to exit, so let the operating system close os.Stdout and os.Stderr.
	buffer := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buffer)
		for pauseOutput.Load() {
			time.Sleep(10 * time.Millisecond)
		}
		if n > 0 {
			buf := buffer[:n]
			if tty {
				detectAndHandleOSC52(buf)
			}
			if win && !tty {
				buf = bytes.ReplaceAll(buf, []byte("\n"), []byte("\r\n"))
			}
			if err := writeAll(writer, buf); err != nil {
				warning("wrap output write failed: %v", err)
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func wrapStdIO(serverIn io.WriteCloser, serverOut, serverErr io.Reader, escapeChar byte, escapeTime time.Duration, sshConn *sshConnection) {
	win := runtime.GOOS == "windows"
	if serverIn != nil {
		go forwardInput(os.Stdin, serverIn, win, escapeChar, escapeTime, sshConn)
	}

	if serverOut != nil {
		outputWaitGroup.Go(func() {
			forwardOutput(serverOut, os.Stdout, win, sshConn.tty)
			debug("ssh session stdout forward completed")
		})
	}
	if serverErr != nil {
		outputWaitGroup.Go(func() {
			beginTime := time.Now()
			forwardOutput(serverErr, os.Stderr, win, sshConn.tty)
			if enableDebugLogging {
				debug("ssh session stderr forward completed")
				if tmuxDebugPaneID == "" && time.Since(beginTime) < 3*time.Second {
					// debug log output may obscure the prompt, attempt to force a screen refresh
					_ = sshConn.session.RedrawScreen(true)
				}
			}
		})
	}
}

func getEscapeConfig(args *sshArgs) (byte, time.Duration) {
	consoleEscapeTime := time.Second
	if t := getExOptionConfig(args, "ConsoleEscapeTime"); t != "" {
		v, err := strconv.ParseUint(t, 10, 32)
		if err != nil {
			warning("ConsoleEscapeTime [%s] is invalid: %v", t, err)
		} else {
			consoleEscapeTime = time.Duration(v) * time.Second
		}
	}

	escapeChar := byte('~')
	if escCh := getOptionConfig(args, "EscapeChar"); escCh != "" {
		if strings.ToLower(escCh) == "none" {
			consoleEscapeTime = 0
		} else if len(escCh) == 2 && escCh[0] == '^' {
			b := escCh[1]
			switch b {
			case 'z', 'Z', 'c', 'C':
				warning("EscapeChar [%s] conflicts with other shortcuts", escCh)
			default:
				if b >= 'a' && b <= 'z' {
					escapeChar = b - 'a' + 1
				} else if b >= 'A' && b <= 'Z' {
					escapeChar = b - 'A' + 1
				} else {
					warning("EscapeChar [%s] is not a valid letter following ^", escCh)
				}
			}
		} else if len(escCh) == 1 {
			b := escCh[0]
			switch b {
			case 'j', 'k', 'q', '.', 'B', 'C', 'R', 'V', 'v', '#', '&', '?', 'd':
				warning("EscapeChar [%s] conflicts with other shortcuts", escCh)
			default:
				if b <= ' ' || b > '~' {
					warning("EscapeChar [%s] is not a valid visible character", escCh)
				} else {
					escapeChar = b
				}
			}
		} else {
			warning("EscapeChar [%s] is not a single character or ‘^’ followed by a letter", escCh)
		}
	}

	return escapeChar, consoleEscapeTime
}

func forwardStdio(sshConn *sshConnection) {
	// not terminal or not tty
	if !isTerminal || !sshConn.tty {
		wrapStdIO(sshConn.serverIn, sshConn.serverOut, sshConn.serverErr, 0, 0, sshConn)
		return
	}

	escapeChar, consoleEscapeTime := getEscapeConfig(sshConn.param.args)
	wrapStdIO(sshConn.serverIn, sshConn.serverOut, sshConn.serverErr, escapeChar, consoleEscapeTime, sshConn)
}
