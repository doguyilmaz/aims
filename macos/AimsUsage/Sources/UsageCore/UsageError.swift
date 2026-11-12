import Foundation

public enum UsageError: LocalizedError, Sendable {
    case unsupportedSchema
    case missingCLI
    case commandFailed(Int32)
    case outputTooLarge
    case timedOut

    public var errorDescription: String? {
        switch self {
        case .unsupportedSchema: "Update Aims Usage to read this usage format."
        case .missingCLI: "Choose the aims executable in Settings, or build the bundled app."
        case .commandFailed(let code): "aims exited with status \(code). Check aims usage --json in Terminal."
        case .outputTooLarge: "The usage response exceeded 4 MB."
        case .timedOut: "The usage refresh timed out. Your last reading is retained."
        }
    }
}
