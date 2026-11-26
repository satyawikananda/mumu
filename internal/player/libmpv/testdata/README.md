# Synthetic HLS segment

`synthetic.mpegts` contains an original two-second test pattern and a 440 Hz tone.
It contains no third-party broadcast material. Tests serve it over loopback
with HLS discontinuities and explicit private-address consent.

Reproduce with FFmpeg:

```sh
ffmpeg -f lavfi -i testsrc2=size=160x90:rate=25 \
  -f lavfi -i sine=frequency=440:sample_rate=48000 -t 2 \
  -c:v mpeg2video -b:v 150k -g 25 -c:a mp2 -b:a 64k \
  -f mpegts synthetic.mpegts
```

The fixture uses MPEG-2 video and MP2 audio, both available in the LGPL build.
It tests headless decoding and control, not native window presentation.
