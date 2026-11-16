import SwiftUI

final class UsageOverviewHostingController: NSHostingController<AnyView> {
    var onCancel: (@MainActor () -> Void)?

    override func cancelOperation(_ sender: Any?) { onCancel?() }
}
