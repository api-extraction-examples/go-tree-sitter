// C# 13 features (.NET 9, November 2024)

using System;
using System.Collections.Generic;

namespace TestApp;

// params with collection types (semantic extension, but should parse correctly)
public class ParamsExample
{
    public void PrintAll(params IEnumerable<string> items)
    {
        foreach (var item in items)
        {
            Console.WriteLine(item);
        }
    }

    public void PrintSpan(params ReadOnlySpan<int> values)
    {
    }
}

// Partial properties
public partial class PartialProps
{
    public partial string Name { get; set; }
}

public partial class PartialProps
{
    private string _name = "";
    public partial string Name
    {
        get => _name;
        set => _name = value ?? throw new ArgumentNullException(nameof(value));
    }
}
