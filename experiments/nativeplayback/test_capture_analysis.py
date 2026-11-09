"""Calibrate the diagnostic analyzer against a known A/V offset and interruption."""
import pathlib
import shutil
import statistics
import subprocess
import tempfile
import unittest

from analyze_capture import analyze


@unittest.skipUnless(shutil.which("ffmpeg") and shutil.which("ffprobe"), "FFmpeg is required")
class CaptureCalibrationTest(unittest.TestCase):
    def test_known_delay_mute_and_black_interval(self):
        with tempfile.TemporaryDirectory(prefix="mumu-calibration-") as directory:
            movie = pathlib.Path(directory) / "known-delay.mov"
            audio = (
                r"aevalsrc=0.02*sin(2*PI*220*t)+"
                r"if(lt(mod(t\,1)\,0.05)\,0.2*sin(2*PI*880*t)\,0):s=48000"
            )
            video = (
                r"drawbox=x=0:y=0:w=40:h=40:color=white:t=fill:enable=lt(mod(t\,1)\,0.05),"
                r"drawbox=color=black:t=fill:enable=between(t\,2.2\,2.4)"
            )
            subprocess.run([
                "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
                "-itsoffset", "0.12", "-f", "lavfi", "-i", audio,
                "-vf", video, "-af", "volume=0:enable='between(t,1.5,1.7)'",
                "-t", "3.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", str(movie),
            ], check=True, timeout=30)
            result = analyze(movie)
            offsets = [pair["audio_minus_video_ms"] for pair in result["paired_sync"]]
            self.assertAlmostEqual(statistics.median(offsets), 120, delta=40)
            self.assertTrue(any(0.15 < end - start < 0.26
                                for start, end in result["low_audio_intervals"]))
            self.assertTrue(any(0.15 < end - start < 0.26
                                for start, end in result["video_absent_intervals"]))
            self.assertFalse(result["repeated_image_intervals_over_100ms"])


if __name__ == "__main__":
    unittest.main()
