import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Avatar, AvatarFallback } from "./avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./dropdown-menu";
import { Label } from "./label";
import { Separator } from "./separator";
import { Sheet, SheetContent, SheetTitle } from "./sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip";

describe("shadcn primitives", () => {
  it("Label associates with control", () => {
    render(
      <div>
        <Label htmlFor="org-name">Name</Label>
        <input id="org-name" />
      </div>
    );
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
  });

  it("Separator renders (decorative by default, accessible when non-decorative)", () => {
    const { unmount } = render(<Separator />);
    // decorative separator is present in the DOM
    expect(document.querySelector('[data-orientation="horizontal"]')).toBeInTheDocument();
    unmount();
    render(<Separator decorative={false} />);
    expect(screen.getByRole("separator")).toBeInTheDocument();
  });

  it("Avatar shows fallback initials", () => {
    render(
      <Avatar>
        <AvatarFallback>AC</AvatarFallback>
      </Avatar>
    );
    expect(screen.getByText("AC")).toBeInTheDocument();
  });

  it("Tabs switches panels", async () => {
    const user = userEvent.setup();
    render(
      <Tabs defaultValue="projects">
        <TabsList>
          <TabsTrigger value="projects">Projects</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>
        <TabsContent value="projects">Projects panel</TabsContent>
        <TabsContent value="settings">Settings panel</TabsContent>
      </Tabs>
    );
    expect(screen.getByText("Projects panel")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Settings" }));
    expect(screen.getByText("Settings panel")).toBeInTheDocument();
  });

  it("DropdownMenu opens and selects item", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(
      <DropdownMenu>
        <DropdownMenuTrigger>Open</DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem onSelect={onSelect}>Sign out</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
    await user.click(screen.getByText("Open"));
    await user.click(await screen.findByText("Sign out"));
    expect(onSelect).toHaveBeenCalled();
  });

  it("Sheet renders title when open", () => {
    render(
      <Sheet open>
        <SheetContent side="left">
          <SheetTitle>Navigation</SheetTitle>
        </SheetContent>
      </Sheet>
    );
    expect(screen.getByText("Navigation")).toBeInTheDocument();
  });

  it("Tooltip shows content on hover", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger>Tables</TooltipTrigger>
          <TooltipContent>Connect a database first</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
    await user.hover(screen.getByText("Tables"));
    expect(await screen.findByText("Connect a database first")).toBeInTheDocument();
  });

  it("Table renders rows accessibly", () => {
    render(
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>Acme</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    );
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("Acme")).toBeInTheDocument();
  });
});
