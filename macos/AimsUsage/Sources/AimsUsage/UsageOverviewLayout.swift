import Foundation

enum UsageOverviewLayout {
    static func width(in visibleFrame: CGRect, style: UsageDisplayStyle) -> CGFloat {
        min(style.preferredWidth, max(0, visibleFrame.width - 24))
    }

    static func maximumBodyHeight(in visibleFrame: CGRect, style: UsageDisplayStyle) -> CGFloat {
        max(0, visibleFrame.height - 24 - style.chromeHeight)
    }
}
