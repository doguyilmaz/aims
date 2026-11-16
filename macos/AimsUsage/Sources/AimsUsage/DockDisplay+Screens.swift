import AppKit

extension DockDisplay {
    static func connected() -> [Self] {
        // NSScreen instances and their geometry may become stale after reconnecting
        // or rearranging displays. Always read the current list at the point of use.
        NSScreen.screens.compactMap { screen in
            guard let number = screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber else {
                return nil
            }
            let uuid = CGDisplayCreateUUIDFromDisplayID(number.uint32Value)?.takeRetainedValue()
            let id = uuid.map { CFUUIDCreateString(nil, $0) as String } ?? "session:\(number.uint32Value)"
            return Self(id: id, name: screen.localizedName, frame: screen.frame, visibleFrame: screen.visibleFrame)
        }
    }
}
