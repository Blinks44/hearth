package inventory

// MobileState is how iOS and Android development is set up: Xcode and
// its simulators, Android's SDK and emulator images, CocoaPods and
// Flutter.
type MobileState struct {
	// Xcodes are the Xcode apps found, on macOS only.
	Xcodes []XcodeApp `json:",omitempty"`
	// Runtimes are the simulators' runtimes installed, on macOS only.
	Runtimes []SimRuntime `json:",omitempty"`
	// Android is the Android SDK; nil when none is found.
	Android *AndroidSDK `json:",omitempty"`
	// CocoaPods is the pod command the terminal runs; nil when none.
	CocoaPods *Installation `json:",omitempty"`
	// Flutter is the flutter command the terminal runs; nil when none.
	Flutter *Flutter `json:",omitempty"`
}

// XcodeApp is one Xcode, by its version.plist.
type XcodeApp struct {
	Path, Version, Build string
	// Selected reports that xcode-select points into it.
	Selected bool `json:",omitempty"`
	// AppStore reports that it came from the App Store.
	AppStore bool `json:",omitempty"`
}

// SimRuntime is a simulator runtime, as iOS 18.2.
type SimRuntime struct {
	Platform, Version string
	// Path is its .simruntime bundle.
	Path string
	// ID is CoreSimulator's runtime UUID from images.plist, when known.
	ID string `json:",omitempty"`
}

// AndroidSDK is an Android SDK folder and what is installed in it.
type AndroidSDK struct {
	Root string
	// Var is the variable that names Root, ANDROID_HOME or
	// ANDROID_SDK_ROOT; empty when Root is the default folder.
	Var string `json:",omitempty"`
	// Missing reports that no folder is at Root.
	Missing bool `json:",omitempty"`
	// Studio is the Android Studio app found, empty when none.
	Studio string `json:",omitempty"`
	// Platforms are the SDK's platforms, as android-35.
	Platforms []AndroidPart `json:",omitempty"`
	// Images are the emulator's system images, each with the AVDs that
	// use it.
	Images []AndroidPart `json:",omitempty"`
}

// AndroidPart is a platform or a system image of the SDK.
type AndroidPart struct {
	// Name is android-35, or an image's API, tag and ABI:
	// android-34/google_apis/arm64-v8a.
	Name string
	API  int
	Path string
	// AVDs are the virtual devices made with an image.
	AVDs []string `json:",omitempty"`
}

// Flutter is a Flutter SDK by its command.
type Flutter struct {
	Installation
	// Root is the SDK's folder; Channel is stable, beta or main.
	Root    string
	Channel string `json:",omitempty"`
}
