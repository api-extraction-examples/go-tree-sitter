// C# 12 features (.NET 8, November 2023)

using System;
using System.Collections.Generic;

namespace TestApp;

// Semicolon-terminated type declarations (bodyless types)
public class BasePluginController : ControllerBase;
public class BasePaymentController : ControllerBase;
public class BaseShippingController : ControllerBase;

public struct EmptyStruct;
public record EmptyRecord;
public record struct EmptyRecordStruct;

// Semicolon-terminated with generic base
public class GenericController<T> : BaseController<T>;

// Semicolon-terminated with multiple interfaces
public class MultiImpl : IDisposable, IComparable;

// Primary constructors on classes and structs
public class UserService(ILogger logger, IRepository repo)
{
    public void Log(string message) => logger.Log(message);
}

public struct Point(double x, double y)
{
    public double X { get; } = x;
    public double Y { get; } = y;
}

// Primary constructor with base invocation and semicolon body
public class DerivedService(ILogger logger) : BaseService(logger);

// Default lambda parameters
public class LambdaDefaults
{
    public void Example()
    {
        var greet = (string name = "World") => $"Hello, {name}!";
    }
}

// using alias for any type (tuple, pointer, etc.)
using Measurement = (double Value, string Unit);
