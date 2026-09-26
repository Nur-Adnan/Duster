using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;

namespace Duster.Core.ViewModels;

/// <summary>A row the user can tick. Concrete rows exist because x:DataType cannot be generic.</summary>
public abstract partial class SelectableRow : ObservableObject
{
    [ObservableProperty]
    public partial bool IsSelected { get; set; }

    /// <summary>What this row's result was (freed, kept, failed…), after an action.</summary>
    [ObservableProperty]
    public partial string Outcome { get; set; } = "";

    public virtual bool CanSelect => true;
    public virtual long Size => 0;
    public string SizeText => Format.Bytes(Size);
}

/// <summary>Rows plus a live "n selected · size" line.</summary>
public sealed class RowList<T> : ObservableCollection<T> where T : SelectableRow
{
    public event Action? SelectionChanged;

    public IReadOnlyList<T> Selected => this.Where(r => r.IsSelected && r.CanSelect).ToList();

    public string Summary => Selected is { Count: > 0 } s ? $"{s.Count} selected · {Format.Bytes(s.Sum(r => r.Size))}" : "Nothing selected";

    public void Replace(IEnumerable<T> rows)
    {
        Clear();
        foreach (var r in rows)
        {
            Add(r);
        }
        SelectionChanged?.Invoke();
    }

    public void SelectAll(bool selected)
    {
        foreach (var r in this.Where(r => r.CanSelect))
        {
            r.IsSelected = selected;
        }
    }

    protected override void InsertItem(int index, T item)
    {
        item.PropertyChanged += OnRowChanged;
        base.InsertItem(index, item);
    }

    protected override void RemoveItem(int index)
    {
        this[index].PropertyChanged -= OnRowChanged;
        base.RemoveItem(index);
        SelectionChanged?.Invoke();
    }

    protected override void ClearItems()
    {
        foreach (var r in this)
        {
            r.PropertyChanged -= OnRowChanged;
        }
        base.ClearItems();
    }

    private void OnRowChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(SelectableRow.IsSelected))
        {
            SelectionChanged?.Invoke();
        }
    }
}
