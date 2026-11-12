#import <Cocoa/Cocoa.h>

@interface DatatreeAppDelegate : NSObject <NSApplicationDelegate>
@property (strong) NSTask *serverTask;
@property (copy) NSString *pendingTarget;
@property (copy) NSString *activeURL;
@property (assign) BOOL hasLaunched;
@property (strong) NSFileHandle *logHandle;
@end

@implementation DatatreeAppDelegate

- (void)setupMenuBar {
    NSMenu *mainMenu = [[NSMenu alloc] initWithTitle:@"MainMenu"];
    NSMenuItem *appMenuItem = [[NSMenuItem alloc] initWithTitle:@"Datatree" action:nil keyEquivalent:@""];
    NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"Datatree"];
    NSMenuItem *openItem = [[NSMenuItem alloc] initWithTitle:@"Open Browser" action:@selector(openActiveURL:) keyEquivalent:@"o"];
    openItem.target = self;
    [appMenu addItem:openItem];
    [appMenu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Quit Datatree" action:@selector(terminate:) keyEquivalent:@"q"];
    [appMenu addItem:quitItem];
    appMenuItem.submenu = appMenu;
    [mainMenu addItem:appMenuItem];
    [NSApp setMainMenu:mainMenu];
}

- (NSString *)firstCLIArgumentTarget {
    NSArray<NSString *> *args = [[NSProcessInfo processInfo] arguments];
    for (NSUInteger i = 1; i < args.count; i++) {
        NSString *arg = args[i];
        if ([arg hasPrefix:@"-psn_"] || [arg hasPrefix:@"-Apple"] || [arg hasPrefix:@"-NS"]) {
            // Ignore macOS LaunchServices and Cocoa launch flags.
        } else {
            return arg;
        }
    }
    return nil;
}

- (NSString *)normalizeOpenedURL:(NSURL *)url {
    NSString *abs = url.absoluteString;
    if (abs == nil) {
        return nil;
    } else if ([url.scheme.lowercaseString isEqualToString:@"datatree"]) {
        NSString *prefix = @"datatree://";
        if (abs.length > prefix.length) {
            return [abs substringFromIndex:prefix.length];
        } else {
            return nil;
        }
    } else if ([url.scheme.lowercaseString isEqualToString:@"file"]) {
        return url.path;
    } else {
        return abs;
    }
}

- (NSDictionary<NSString *, NSString *> *)childEnvironment {
    NSMutableDictionary<NSString *, NSString *> *env = [[[NSProcessInfo processInfo] environment] mutableCopy];
    NSString *home = NSHomeDirectory();
    NSString *existingPath = env[@"PATH"] ?: @"/usr/bin:/bin:/usr/sbin:/sbin";
    NSString *extraPath = [NSString stringWithFormat:@"/opt/homebrew/bin:/usr/local/bin:%@/.local/bin:%@", home, existingPath];
    env[@"PATH"] = extraPath;
    return env;
}

- (void)openLogFile {
    if (self.logHandle != nil) {
        return;
    } else {
        NSString *logsDir = [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Logs"];
        [[NSFileManager defaultManager] createDirectoryAtPath:logsDir withIntermediateDirectories:YES attributes:nil error:nil];
        NSString *logPath = [logsDir stringByAppendingPathComponent:@"Datatree.log"];
        if (![[NSFileManager defaultManager] fileExistsAtPath:logPath]) {
            [[NSFileManager defaultManager] createFileAtPath:logPath contents:nil attributes:nil];
        } else {
            // Append to existing Datatree.log file.
        }
        self.logHandle = [NSFileHandle fileHandleForWritingAtPath:logPath];
        if (self.logHandle != nil) {
            [self.logHandle seekToEndOfFile];
        } else {
            // Proceed even if log file could not be opened.
        }
    }
}

- (void)handleOutputData:(NSData *)data {
    if (data.length == 0) {
        return;
    } else {
        if (self.logHandle != nil) {
            [self.logHandle writeData:data];
        } else {
            // No log file handle open.
        }
        NSString *text = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        if (text != nil) {
            for (NSString *line in [text componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]]) {
                NSRange range = [line rangeOfString:@"Serving at http://"];
                if (range.location != NSNotFound) {
                    NSString *urlStr = [[line substringFromIndex:range.location + @"Serving at ".length] stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
                    if (urlStr.length > 0) {
                        dispatch_async(dispatch_get_main_queue(), ^{
                            self.activeURL = urlStr;
                        });
                    } else {
                        // Ignore empty URL suffix.
                    }
                } else {
                    // Other log line recorded only.
                }
            }
        } else {
            // Non-UTF8 output logged as raw bytes.
        }
    }
}

- (void)stopServerTask {
    if (self.serverTask != nil) {
        if (self.serverTask.isRunning) {
            [self.serverTask terminate];
            [self.serverTask waitUntilExit];
        } else {
            // Task already exited.
        }
        self.serverTask = nil;
    } else {
        // No active server task to stop.
    }
}

- (void)launchDatatreeWithTarget:(NSString *)target {
    [self stopServerTask];
    [self openLogFile];

    NSString *binaryPath = [[NSBundle mainBundle] pathForAuxiliaryExecutable:@"datatree-bin"];
    if (binaryPath == nil || ![[NSFileManager defaultManager] isExecutableFileAtPath:binaryPath]) {
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = @"Datatree executable missing";
        alert.informativeText = @"Expected Contents/MacOS/datatree-bin inside Datatree.app.";
        [alert runModal];
        [NSApp terminate:nil];
        return;
    } else {
        NSTask *task = [[NSTask alloc] init];
        task.executableURL = [NSURL fileURLWithPath:binaryPath];
        if (target != nil && target.length > 0) {
            task.arguments = @[target];
        } else {
            task.arguments = @[];
        }
        task.environment = [self childEnvironment];

        NSPipe *pipe = [NSPipe pipe];
        task.standardOutput = pipe;
        task.standardError = pipe;
        NSFileHandle *readHandle = pipe.fileHandleForReading;
        __weak typeof(self) weakSelf = self;
        readHandle.readabilityHandler = ^(NSFileHandle *handle) {
            NSData *chunk = [handle availableData];
            if (chunk.length == 0) {
                handle.readabilityHandler = nil;
            } else {
                [weakSelf handleOutputData:chunk];
            }
        };

        task.terminationHandler = ^(NSTask *finishedTask) {
            readHandle.readabilityHandler = nil;
            dispatch_async(dispatch_get_main_queue(), ^{
                if (weakSelf.serverTask == finishedTask && finishedTask.terminationStatus != 0 && finishedTask.terminationReason != NSTaskTerminationReasonUncaughtSignal) {
                    NSAlert *alert = [[NSAlert alloc] init];
                    alert.messageText = @"Datatree exited unexpectedly";
                    alert.informativeText = @"See ~/Library/Logs/Datatree.log for details.";
                    [alert runModal];
                    [NSApp terminate:nil];
                } else {
                    // Normal shutdown or superseded task exit.
                }
            });
        };

        NSError *launchError = nil;
        if (![task launchAndReturnError:&launchError]) {
            NSAlert *alert = [[NSAlert alloc] init];
            alert.messageText = @"Cannot start Datatree";
            alert.informativeText = launchError.localizedDescription ?: @"Unknown launch error";
            [alert runModal];
            [NSApp terminate:nil];
            return;
        } else {
            self.serverTask = task;
        }
    }
}

- (IBAction)openActiveURL:(id)sender {
    if (self.activeURL != nil && self.activeURL.length > 0) {
        NSURL *url = [NSURL URLWithString:self.activeURL];
        if (url != nil) {
            [[NSWorkspace sharedWorkspace] openURL:url];
        } else {
            // Invalid stored URL string.
        }
    } else {
        // Active URL not yet reported.
    }
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    [self setupMenuBar];
    self.hasLaunched = YES;
    NSString *target = self.pendingTarget ?: [self firstCLIArgumentTarget];
    self.pendingTarget = nil;
    [self launchDatatreeWithTarget:target];
}

- (BOOL)application:(NSApplication *)sender openFile:(NSString *)filename {
    if (!self.hasLaunched) {
        self.pendingTarget = filename;
    } else {
        [self launchDatatreeWithTarget:filename];
    }
    return YES;
}

- (void)application:(NSApplication *)sender openFiles:(NSArray<NSString *> *)filenames {
    if (filenames.count > 0) {
        [self application:sender openFile:filenames.firstObject];
        [sender replyToOpenOrPrint:NSApplicationDelegateReplySuccess];
    } else {
        [sender replyToOpenOrPrint:NSApplicationDelegateReplyFailure];
    }
}

- (void)application:(NSApplication *)application openURLs:(NSArray<NSURL *> *)urls {
    if (urls.count > 0) {
        NSString *target = [self normalizeOpenedURL:urls.firstObject];
        if (target != nil && target.length > 0) {
            if (!self.hasLaunched) {
                self.pendingTarget = target;
            } else {
                [self launchDatatreeWithTarget:target];
            }
        } else {
            // Empty URL target ignored.
        }
    } else {
        // No URLs provided.
    }
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
    [self openActiveURL:sender];
    return YES;
}

- (void)applicationWillTerminate:(NSNotification *)notification {
    [self stopServerTask];
    if (self.logHandle != nil) {
        [self.logHandle closeFile];
        self.logHandle = nil;
    } else {
        // No log handle to close.
    }
}

@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyRegular];
        DatatreeAppDelegate *delegate = [[DatatreeAppDelegate alloc] init];
        app.delegate = delegate;
        [app run];
    }
    return 0;
}
