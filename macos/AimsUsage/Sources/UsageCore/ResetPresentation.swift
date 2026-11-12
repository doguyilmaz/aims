import Foundation

public enum ResetPresentation {
    public static func countdown(to date: Date, now: Date) -> String {
        let seconds = date.timeIntervalSince(now)
        guard seconds > 0 else { return "Refresh needed" }
        let minutes = max(1, Int(ceil(seconds / 60)))
        if minutes < 60 { return "in \(minutes)m" }
        let hours = minutes / 60
        if hours < 24 { return "in \(hours)h \(minutes % 60)m" }
        return "in \(hours / 24)d \(hours % 24)h"
    }

    public static func period(_ label: String) -> String {
        switch label {
        case "5h": "5 hours"
        case "7d": "Weekly"
        case "1d": "Daily"
        case "month": "Monthly"
        case "budget", "set": "Budget"
        default: label
        }
    }
}
