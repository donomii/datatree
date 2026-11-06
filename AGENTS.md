# Agent Notes

You are not allowed to change the formatting.

Agent workflow:
- Before anything else, read the SPEC.md
- After making changes based on user preferences, update the SPEC.md to reflect the new behavior.

# SPEC.md

The SPEC file is a very important document that describes, in detail, everything about this project in enough detail that the entire project can be re-created entirely from the spec.  The therefore must include:

- A solid summary of the project
- a detailed description of all user interactions.  Every window, button, click, response, and behaviour must be documented in detail.
- The exact layout is not important.  If the spec says "and the user clicks the OK button to continue", you can put the OK button wherever you want.  The important thing is that the user can continue.
- It must contain a description of all major datatypes used in the program, with detailed annotations on their use.  Major means global datatypes, and datatypes that are used for interacting with other systems (API calls, FFI calls, database calls, etc.
- Objects must document their public methods and fields.  Private methods are not documented unless they are particularly complex or important.
- Small, simple helper functions are not documented unless they are particularly complex or important.
- Temporary variables or functions are not documented.  e.g. a variable only used inside a single function is not documented.
- If the program expects files to be on disk, e.g. picture files or .config files, the spec must describe the expected file format, and where the files are expected to be found, in addition to all the other requirements above (i.e. external data types, etc.)
- When describing functions, objects, methods, and datatypes, don't describe them in terms of a specific programming language, unless that is vital to the function of the program.  e.g. specify numbers as floats, integers, etc. not as int16, float64, etc.  Strings are just strings, not byte arrays, or UTF-8, etc.
- if an algorithm or data type requires a specific type, like a 8 bit integer, or a 32 bit float, then be sure to specify that, and the reason why it is required.
- Algorithms should be described in terms of pseudocode, not in terms of a specific programming language.

# git

you are forbidden from using any destructive git commands, at all.  You are double definitely forbidden from using destructive git commands.

# Coding Style

You will not change the formatting.

## Every if statement must have an else

Every if statement, or other conditional statement, must have an else.  All outcomes must be dealt with.  No "fall through" logic.

You will not alter the number of spaces or tabs at the start of a line.

## File Organization

Group by function, files should contain functions that call each other, and/or do similar things or act on similar data.

unless

functions are part of an object.  Objects should go in their own file, especially if they are large.  Ideally files should stay under 1000 lines

High cohesion within files, loose coupling between files
- 200-400 lines typical, 800 max
- Extract utilities from large components
- Organize by feature/domain, not by type

You will not add or remove any whitespace, anywhere in the code.


## Error Handling

return errors if the condition is expected, and execution will normally continue afterwards.  examples:

err = openfile() - expect file not to exist
sendToServer() - server or network is expected to fail sometimes

if the condition is unexpected or severe use a panic or throw an error

if !closeFile() { panic }


In general, prefer a panic over an error, unless you are very sure you are going to deal with the error immediately, and effectively.

I didn't think I had to explain this, but apparently this is a difficult concept.  YOU HAVE TO CATCH THE PANIC.

Choose an appropriate point to catch the panic, and deal with it.  If you are processing an array, you can catch the panic, then move on to the next item.  Or, you can abort the entire array.  The strategy is entirely customised to the circumstances.  In general, you should find a 'safe point' to catch the panic, and deal with it. 

## Error reporting

When writing error messages, and log files, errors must be as complete and detailed as possible, returning enough information to debug them just be reading them.  Errors must contain: what the result was, the expected result, the task being attempted, and any other supporting data in the function.

Error messages must be comprehensible, numeric errors are not allowed "Error: code 58" is completely unacceptable.

do not change the formatting of the code.

## Locks

Locks should only be used in pairs, like this

l.Lock()
defer l.Unlock()

Never lock and unlock in different functions.

Locks should only be used to lock datastructures, in functions that don't call other functions.  e.g.

data object

method set
lock
defer unlock

set value
return

method get
lock
defer unlock

get value
return value


Formatting changes are verboten.

----

if the language you are using does not have defer, try using a thunk

method set

f = thunk {
   set value
}
lock
f()
check error
unlock
return

This prevents someone add an extra return statement in a function and skipping the unlock.

----

Locks should never appear in the main package, or the top level of other packages.

---

If you need a synchronised map, use https://github.com/donomii/genericsyncmap

## Types

Do not use void*, interface{}, or any other "escape type"

----

## Readme.md

If a README.md already exists, you will not touch it, or edit it in anyway.  It is strictly forbidden to modify it.

If the readme does not exist, you will create one.  

when you create one it will contain a project introduction, and instructions on how to build and run the project.  

The introduction must be written in a way that is accessible to someone who is not familiar with the project.  It explains what the project does, and why someone would want to use it.  

## Code comments.

You will not remove any comments from the code, at all, for any reason.

## Convenience scripts

You will produce build.sh, run.sh, test.sh, demo.sh and install.sh scripts.  You will not just tell the user to run a command, that command goes in a script.

If the software does additional things, like rendering, or exporting, you will produce additional scripts to demonstrate those features.

## Check the job is done

If you are writing software, you must compile and run the software to ensure that it works correctly.  You will not report to the user that the task is finished until you have done this.

In order to do this, you should add a -test flag, which will run the software through a reasonable series of tests that exercise the basic functionality of the software, plus whatever new functionality you have added.

You can then compile and run -test the software to ensure that it works correctly, and after it does, you can report to the user that the task is finished.


## Code Quality Checklist

Before marking work complete:
- [ ] Code is readable and well-named
- [ ] Functions are small (<50 lines)
- [ ] Files are focused (<800 lines)
- [ ] No deep nesting (>4 levels)
- [ ] Proper error handling
- [ ] No hardcoded values
- [ ] No void* or similar types
- [ ] SPEC.md updated
- [ ] Work is not complete until the code builds and tests are run
- [ ] No formatting changes
