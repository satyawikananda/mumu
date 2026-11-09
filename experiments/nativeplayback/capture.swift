// Diagnostic recorder only. It captures one explicitly selected process, never the microphone.
import AVFoundation
import AppKit
import CoreGraphics
import Foundation
import ScreenCaptureKit

final class Recorder: NSObject, SCStreamOutput, SCStreamDelegate {
    let queue = DispatchQueue(label: "mumu.capture.samples")
    let writer: AVAssetWriter
    let video: AVAssetWriterInput
    let audio: AVAssetWriterInput
    var started = false
    var videoCount = 0
    var audioCount = 0
    var droppedCount = 0
    var streamError: Error?

    init(url: URL, width: Int, height: Int) throws {
        writer = try AVAssetWriter(outputURL: url, fileType: .mov)
        video = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264,
            AVVideoWidthKey: width, AVVideoHeightKey: height,
            AVVideoCompressionPropertiesKey: [AVVideoAverageBitRateKey: 6_000_000]
        ])
        audio = AVAssetWriterInput(mediaType: .audio, outputSettings: [
            AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 48_000,
            AVNumberOfChannelsKey: 2, AVEncoderBitRateKey: 192_000
        ])
        super.init()
        video.expectsMediaDataInRealTime = true
        audio.expectsMediaDataInRealTime = true
        guard writer.canAdd(video), writer.canAdd(audio) else {
            throw NSError(domain: "mumu.capture", code: 1,
                          userInfo: [NSLocalizedDescriptionKey: "Cannot add capture tracks"])
        }
        writer.add(video)
        writer.add(audio)
    }

    func stream(_ stream: SCStream, didStopWithError error: Error) {
        queue.async { self.streamError = error }
    }

    func stream(_ stream: SCStream, didOutputSampleBuffer sample: CMSampleBuffer,
                of type: SCStreamOutputType) {
        guard CMSampleBufferIsValid(sample) else { return }
        if type == .screen {
            guard let attachments = CMSampleBufferGetSampleAttachmentsArray(
                sample, createIfNecessary: false) as? [[SCStreamFrameInfo: Any]],
                let status = attachments.first?[.status] as? Int,
                status == SCFrameStatus.complete.rawValue else { return }
        }
        if !started {
            guard type == .screen, writer.startWriting() else { return }
            writer.startSession(atSourceTime: CMSampleBufferGetPresentationTimeStamp(sample))
            let delay = CMTimeGetSeconds(CMTimeSubtract(
                CMClockGetTime(CMClockGetHostTimeClock()), CMSampleBufferGetPresentationTimeStamp(sample)))
            print("CAPTURE_ORIGIN unix=\(Date().timeIntervalSince1970 - delay)")
            started = true
        }
        let input = type == .screen ? video : audio
        guard input.isReadyForMoreMediaData else { droppedCount += 1; return }
        guard input.append(sample) else { streamError = writer.error; return }
        if type == .screen { videoCount += 1 } else { audioCount += 1 }
    }

    func finish() async throws {
        let failure: Error? = queue.sync {
            video.markAsFinished()
            audio.markAsFinished()
            return streamError
        }
        if let failure { throw failure }
        guard started else {
            throw NSError(domain: "mumu.capture", code: 2,
                          userInfo: [NSLocalizedDescriptionKey: "No complete video samples"])
        }
        await writer.finishWriting()
        if let failure = writer.error { throw failure }
        guard videoCount > 0, audioCount > 0, droppedCount == 0 else {
            throw NSError(domain: "mumu.capture", code: 3, userInfo: [
                NSLocalizedDescriptionKey:
                    "Invalid capture: video=\(videoCount) audio=\(audioCount) dropped=\(droppedCount)"
            ])
        }
        print("CAPTURE_OK video=\(videoCount) audio=\(audioCount) dropped=\(droppedCount)")
    }
}

@main struct Capture {
    static func main() async {
        do {
            _ = NSApplication.shared
            NSApplication.shared.setActivationPolicy(.prohibited)
            let args = CommandLine.arguments
            if args.count == 2 && args[1] == "--request-access" {
                print("screen_capture_allowed=\(CGRequestScreenCaptureAccess())")
                return
            }
            guard (4...5).contains(args.count), let pid = Int32(args[1]), pid > 0,
                  let seconds = UInt64(args[3]), (1...120).contains(seconds) else {
                throw NSError(domain: "mumu.capture", code: 4, userInfo: [
                    NSLocalizedDescriptionKey: "usage: mumu-capture PID NEW_OUTPUT.mov SECONDS(1..120) [EXACT_WINDOW_TITLE]"
                ])
            }
            guard CGPreflightScreenCaptureAccess() else {
                fputs("CAPTURE_PERMISSION_REQUIRED: Screen & System Audio Recording\n", stderr)
                exit(3)
            }
            var content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: false)
            for _ in 0..<40 {
                let available = content.windows.contains {
                    $0.owningApplication?.processID == pid && $0.frame.width >= 300 && $0.frame.height >= 200
                }
                if available { break }
                try await Task.sleep(nanoseconds: 250_000_000)
                content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: false)
            }
            guard let app = content.applications.first(where: { $0.processID == pid }),
                  let display = content.displays.first else {
                throw NSError(domain: "mumu.capture", code: 5, userInfo: [
                    NSLocalizedDescriptionKey: "Selected process or display is unavailable"
                ])
            }
            let windows = content.windows.filter { $0.owningApplication?.processID == pid }
            for window in windows {
                print("TARGET_WINDOW id=\(window.windowID) title=\(window.title ?? "") frame=\(window.frame)")
            }
            let filter: SCContentFilter
            if args.count == 5 {
                guard let window = windows.first(where: { $0.title == args[4] }) else {
                    throw NSError(domain: "mumu.capture", code: 6, userInfo: [
                        NSLocalizedDescriptionKey: "Requested process window is unavailable"
                    ])
                }
                filter = SCContentFilter(desktopIndependentWindow: window)
            } else {
                filter = SCContentFilter(display: display, including: [app], exceptingWindows: [])
            }
            let configuration = SCStreamConfiguration()
            configuration.width = display.width
            configuration.height = display.height
            configuration.minimumFrameInterval = CMTime(value: 1, timescale: 60)
            configuration.queueDepth = 5
            configuration.showsCursor = false
            configuration.capturesAudio = true
            configuration.sampleRate = 48_000
            configuration.channelCount = 2
            configuration.excludesCurrentProcessAudio = true
            let url = URL(fileURLWithPath: args[2])
            let recorder = try Recorder(url: url, width: display.width, height: display.height)
            let stream = SCStream(filter: filter, configuration: configuration, delegate: recorder)
            try stream.addStreamOutput(recorder, type: .screen, sampleHandlerQueue: recorder.queue)
            try stream.addStreamOutput(recorder, type: .audio, sampleHandlerQueue: recorder.queue)
            try await stream.startCapture()
            print("CAPTURE_STARTED pid=\(pid) unix=\(Date().timeIntervalSince1970) width=\(display.width) height=\(display.height)")
            fflush(stdout)
            try await Task.sleep(nanoseconds: seconds * 1_000_000_000)
            try await stream.stopCapture()
            try await recorder.finish()
        } catch {
            fputs("CAPTURE_FAILED: \(error)\n", stderr)
            exit(1)
        }
    }
}
