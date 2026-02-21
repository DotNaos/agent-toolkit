import AppKit
import ApplicationServices
import Foundation

struct Bounds: Codable {
    let x: Double
    let y: Double
    let width: Double
    let height: Double
}

struct AXNode: Codable {
    let role: String?
    let subrole: String?
    let title: String?
    let value: String?
    let description: String?
    let identifier: String?
    let enabled: Bool?
    let focused: Bool?
    let bounds: Bounds?
    let children: [AXNode]
}

struct AXDumpOutput: Codable {
    let status: String
    let appName: String?
    let pid: Int32?
    let generatedAt: String
    let root: AXNode?
    let message: String?
}

let isoFormatter: ISO8601DateFormatter = {
    let f = ISO8601DateFormatter()
    f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return f
}()

func attributeValue(_ element: AXUIElement, _ name: CFString) -> AnyObject? {
    var value: CFTypeRef?
    let err = AXUIElementCopyAttributeValue(element, name, &value)
    guard err == .success, let value else {
        return nil
    }
    return value
}

func stringAttribute(_ element: AXUIElement, _ name: CFString) -> String? {
    guard let raw = attributeValue(element, name) else {
        return nil
    }

    if let value = raw as? String {
        return value
    }
    if let value = raw as? NSAttributedString {
        return value.string
    }
    if let value = raw as? NSNumber {
        return value.stringValue
    }
    return nil
}

func boolAttribute(_ element: AXUIElement, _ name: CFString) -> Bool? {
    guard let raw = attributeValue(element, name) else {
        return nil
    }
    if let value = raw as? Bool {
        return value
    }
    if let value = raw as? NSNumber {
        return value.boolValue
    }
    return nil
}

func boundsAttribute(_ element: AXUIElement) -> Bounds? {
    guard
        let posRawObj = attributeValue(element, kAXPositionAttribute as CFString),
        let sizeRawObj = attributeValue(element, kAXSizeAttribute as CFString)
    else {
        return nil
    }

    let posRaw = posRawObj as! AXValue
    let sizeRaw = sizeRawObj as! AXValue

    guard AXValueGetType(posRaw) == .cgPoint, AXValueGetType(sizeRaw) == .cgSize else {
        return nil
    }

    var point = CGPoint.zero
    var size = CGSize.zero
    AXValueGetValue(posRaw, .cgPoint, &point)
    AXValueGetValue(sizeRaw, .cgSize, &size)

    return Bounds(x: point.x, y: point.y, width: size.width, height: size.height)
}

func childrenAttribute(_ element: AXUIElement) -> [AXUIElement] {
    guard let raw = attributeValue(element, kAXChildrenAttribute as CFString) else {
        return []
    }

    if let children = raw as? [AXUIElement] {
        return children
    }

    if let children = raw as? [AnyObject] {
        return children.map { $0 as! AXUIElement }
    }

    return []
}

func valueAttribute(_ element: AXUIElement) -> String? {
    guard let raw = attributeValue(element, kAXValueAttribute as CFString) else {
        return nil
    }

    if let value = raw as? String {
        return value
    }
    if let value = raw as? NSNumber {
        return value.stringValue
    }
    if let value = raw as? NSAttributedString {
        return value.string
    }

    return nil
}

func dumpNode(_ element: AXUIElement, depth: Int, maxDepth: Int, visited: inout Set<UInt>) -> AXNode {
    let pointer = UInt(bitPattern: Unmanaged.passUnretained(element).toOpaque())
    let wasVisited = visited.contains(pointer)
    visited.insert(pointer)

    let role = stringAttribute(element, kAXRoleAttribute as CFString)
    let subrole = stringAttribute(element, kAXSubroleAttribute as CFString)
    let title = stringAttribute(element, kAXTitleAttribute as CFString)
    let value = valueAttribute(element)
    let description = stringAttribute(element, kAXDescriptionAttribute as CFString)
    let identifier = stringAttribute(element, kAXIdentifierAttribute as CFString)
    let enabled = boolAttribute(element, kAXEnabledAttribute as CFString)
    let focused = boolAttribute(element, kAXFocusedAttribute as CFString)
    let bounds = boundsAttribute(element)

    if depth >= maxDepth || wasVisited {
        return AXNode(
            role: role,
            subrole: subrole,
            title: title,
            value: value,
            description: description,
            identifier: identifier,
            enabled: enabled,
            focused: focused,
            bounds: bounds,
            children: []
        )
    }

    let childNodes = childrenAttribute(element).map { child in
        dumpNode(child, depth: depth + 1, maxDepth: maxDepth, visited: &visited)
    }

    return AXNode(
        role: role,
        subrole: subrole,
        title: title,
        value: value,
        description: description,
        identifier: identifier,
        enabled: enabled,
        focused: focused,
        bounds: bounds,
        children: childNodes
    )
}

func emitJSON(_ output: AXDumpOutput, exitCode: Int32) -> Never {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]

    if let data = try? encoder.encode(output), let text = String(data: data, encoding: .utf8) {
        print(text)
    } else {
        print("{\"status\":\"error\",\"message\":\"failed to encode JSON\"}")
    }

    exit(exitCode)
}

if !AXIsProcessTrusted() {
    let promptKey = kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String
    let opts = [promptKey: true] as CFDictionary
    _ = AXIsProcessTrustedWithOptions(opts)

    emitJSON(
        AXDumpOutput(
            status: "error",
            appName: nil,
            pid: nil,
            generatedAt: isoFormatter.string(from: Date()),
            root: nil,
            message: "Accessibility permission missing. Enable access for Terminal/Codex in System Settings > Privacy & Security > Accessibility."
        ),
        exitCode: 1
    )
}

guard let app = NSWorkspace.shared.frontmostApplication else {
    emitJSON(
        AXDumpOutput(
            status: "error",
            appName: nil,
            pid: nil,
            generatedAt: isoFormatter.string(from: Date()),
            root: nil,
            message: "Could not resolve frontmost application."
        ),
        exitCode: 1
    )
}

let appElement = AXUIElementCreateApplication(app.processIdentifier)
var visited = Set<UInt>()
let root = dumpNode(appElement, depth: 0, maxDepth: 8, visited: &visited)

emitJSON(
    AXDumpOutput(
        status: "success",
        appName: app.localizedName,
        pid: app.processIdentifier,
        generatedAt: isoFormatter.string(from: Date()),
        root: root,
        message: nil
    ),
    exitCode: 0
)
