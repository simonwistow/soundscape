// Plays a theme's samples in SuperCollider, from soundscape's OSC output.
//
// Run it with the theme's samples directory, then point soundscape at it,
// with its own sample player off:
//
//   sclang examples/osc/supercollider.scd themes/forest/samples
//   soundscape --theme themes/forest/theme.yaml --output osc:localhost:57120
//
// (sclang is inside SuperCollider.app on macOS:
// /Applications/SuperCollider.app/Contents/MacOS/sclang.) Or open it in the
// SuperCollider IDE, set ~samplesDir below, and run it with Cmd-Return.
//
// Every directory of .wav files under the samples directory is a group,
// named as soundscape names it (e.g. "birds"). A /sample message plays a
// random one of its group's files once, at the pitch, gain and pan it
// gives; a /loop message starts that sound's loop the first time, then
// glides its gain and pan to each new value. Everything else (notes,
// controls, programs) is printed, as a start on doing something with it.

(
~samplesDir = ~samplesDir ?? { thisProcess.argv.first } ?? { "themes/forest/samples" };
~samplesDir = PathName(~samplesDir.standardizePath).fullPath;

// No audio input is needed. The server opens an input device all the same,
// and opening Bluetooth headphones' microphone (AirPods, say) switches them
// to a low-quality call mode at a different sample rate, which can stop
// the server booting or make everything play at the wrong speed. So take
// input from the computer's own microphone, if it has one.
s.options.numInputBusChannels = 0;
ServerOptions.inDevices.detect { |d| d.containsi("MacBook") or: { d.containsi("Built-in") } } !? { |d|
	s.options.inDevice = d;
};

s.waitForBoot {
	SynthDef(\soundscapeShot, { |buf, rate = 1, amp = 0.5, pan = 0, dur = 1|
		// Like soundscape's own player: an 8ms fade in, and out at dur.
		var env = EnvGen.kr(Env.linen(0.008, (dur - 0.016).max(0), 0.008), doneAction: 2);
		var sig = PlayBuf.ar(1, buf, rate * BufRateScale.kr(buf), doneAction: 2);
		Out.ar(0, Pan2.ar(sig * env, pan, amp));
	}).add;

	SynthDef(\soundscapeLoop, { |buf, rate = 1, amp = 0, pan = 0|
		var sig = PlayBuf.ar(1, buf, rate * BufRateScale.kr(buf), loop: 1);
		Out.ar(0, Pan2.ar(sig, Lag.kr(pan, 0.3), Lag.kr(amp, 0.3)));
	}).add;

	~groups = ();
	PathName(~samplesDir).folders.do { |dir|
		var files = dir.files.select { |f| f.extension.toLower == "wav" };
		if (files.notEmpty) {
			~groups[dir.folderName.asSymbol] = files.collect { |f| Buffer.readChannel(s, f.fullPath, channels: [0]) };
		};
	};
	~loops = ();
	s.sync;
	"soundscape: % sample groups from %".format(~groups.size, ~samplesDir).postln;

	thisProcess.removeOSCRecvFunc(~soundscapeRecv);
	~soundscapeRecv = { |msg|
		var parts = msg[0].asString.split($/).reject(_.isEmpty);
		var kind = parts.last.asSymbol, sound = parts[parts.size - 2].asSymbol;
		var bufs;
		if (parts.first == "soundscape") {
			switch(kind,
				\sample, {
					// group, pitch, gain, pan, duration_ms, channel
					bufs = ~groups[msg[1].asSymbol];
					if (bufs.notNil) {
						Synth(\soundscapeShot, [buf: bufs.choose, rate: msg[2], amp: msg[3], pan: msg[4], dur: msg[5] / 1000]);
					};
				},
				\loop, {
					// group, pitch, gain, pan, channel
					if (~loops[sound].notNil) {
						~loops[sound].set(\rate, msg[2], \amp, msg[3], \pan, msg[4]);
					} {
						bufs = ~groups[msg[1].asSymbol];
						if (bufs.notNil) {
							~loops[sound] = Synth(\soundscapeLoop, [buf: bufs.choose, rate: msg[2], amp: msg[3], pan: msg[4]]);
						};
					};
				},
				{ msg.postln }
			);
		};
	};
	thisProcess.addOSCRecvFunc(~soundscapeRecv);
	"soundscape: listening on port %".format(NetAddr.langPort).postln;
};
)
