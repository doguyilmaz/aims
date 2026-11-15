import SwiftUI

struct UsageScrollStyle: NSViewRepresentable {
    @Environment(\.usageSlimScrollbars) private var slim

    func makeNSView(context: Context) -> UsageScrollStyleView { UsageScrollStyleView() }

    func updateNSView(_ view: UsageScrollStyleView, context: Context) {
        view.slim = slim
        view.configureScrollViews()
    }
}
