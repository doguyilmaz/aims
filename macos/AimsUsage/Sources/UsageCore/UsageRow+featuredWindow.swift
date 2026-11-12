import Foundation

extension UsageRow {
    // A passed reset is never a local refill. Keep all windows for details.
    public func featuredWindow(at now: Date) -> UsageWindow? {
        let all = windows ?? []
        let current = all.filter { !$0.needsRefresh(at: now) }
        return (current.isEmpty ? all : current).reduce(nil as UsageWindow?) { best, window in
            guard let best else { return window }
            if (window.limited == true) != (best.limited == true) {
                return window.limited == true ? window : best
            }
            return (window.validPercent ?? -1) > (best.validPercent ?? -1) ? window : best
        }
    }
}
